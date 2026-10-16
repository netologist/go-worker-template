package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/netologist/go-worker-template/internal/domain"
)

const defaultTTL = 7 * 24 * time.Hour

// TaskPayload represents the expected JSON payload format inside an EventMessage.
type TaskPayload struct {
	TaskID  string `json:"task_id,omitempty"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

// ProcessTaskUseCase coordinates idempotency, validation, domain processing, and DLQ dispatch.
type ProcessTaskUseCase struct {
	repo             domain.TaskRepository
	idempotencyStore domain.IdempotencyStore
	dlqPublisher     domain.DeadLetterPublisher
	idempotencyTTL   time.Duration
	logger           *slog.Logger
}

// NewProcessTaskUseCase initializes a new ProcessTaskUseCase.
func NewProcessTaskUseCase(
	repo domain.TaskRepository,
	idempotencyStore domain.IdempotencyStore,
	dlqPublisher domain.DeadLetterPublisher,
	idempotencyTTL time.Duration,
	logger *slog.Logger,
) *ProcessTaskUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	if idempotencyTTL <= 0 {
		idempotencyTTL = defaultTTL
	}

	return &ProcessTaskUseCase{
		repo:             repo,
		idempotencyStore: idempotencyStore,
		dlqPublisher:     dlqPublisher,
		idempotencyTTL:   idempotencyTTL,
		logger:           logger,
	}
}

// Process implements domain.TaskProcessor.
func (uc *ProcessTaskUseCase) Process(ctx context.Context, msg domain.EventMessage) error {
	msgID := msg.Metadata.ID
	if msgID == "" {
		err := fmt.Errorf("%w: missing message ID in metadata", domain.ErrFatal)
		uc.logger.ErrorContext(ctx, "poison pill detected: missing message ID", "error", err)
		if uc.dlqPublisher != nil {
			_ = uc.dlqPublisher.PublishToDLQ(ctx, msg, err)
		}
		return nil
	}

	subLogger := uc.logger.With(
		"message_id", msgID,
		"trace_id", msg.Metadata.TraceID,
		"correlation_id", msg.Metadata.CorrelationID,
		"type", msg.Metadata.Type,
	)

	// Idempotency Check
	proceed, err := uc.acquireIdempotency(ctx, msgID, subLogger)
	if err != nil || !proceed {
		return err
	}

	// Payload & Domain Validation
	task, fatalErr := uc.buildTask(msg)
	if fatalErr != nil {
		subLogger.ErrorContext(ctx, "fatal task validation error", "error", fatalErr)
		return uc.handleFatal(ctx, msg, msgID, fatalErr)
	}

	// Execute Business Operation
	if err := uc.persistAndComplete(ctx, &task, msgID, subLogger); err != nil {
		return err
	}

	subLogger.InfoContext(ctx, "task processed successfully", "task_id", task.ID)
	return nil
}

func (uc *ProcessTaskUseCase) acquireIdempotency(
	ctx context.Context,
	msgID string,
	logger *slog.Logger,
) (bool, error) {
	if uc.idempotencyStore == nil {
		return true, nil
	}

	acquired, err := uc.idempotencyStore.TryAcquire(ctx, msgID, uc.idempotencyTTL)
	if err != nil {
		logger.ErrorContext(ctx, "transient error acquiring idempotency lock", "error", err)
		return false, fmt.Errorf("%w: failed to acquire idempotency lock: %w", domain.ErrTransient, err)
	}
	if !acquired {
		logger.InfoContext(ctx, "message already processed or in-flight; skipping execution")
		return false, nil
	}

	return true, nil
}

func (uc *ProcessTaskUseCase) buildTask(msg domain.EventMessage) (domain.Task, error) {
	if len(msg.Payload) == 0 {
		return domain.Task{}, fmt.Errorf("%w: empty message payload", domain.ErrFatal)
	}

	var payload TaskPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return domain.Task{}, fmt.Errorf("%w: malformed JSON payload: %w", domain.ErrFatal, err)
	}

	taskID := uuid.New()
	if payload.TaskID != "" {
		parsedUUID, err := uuid.Parse(payload.TaskID)
		if err != nil {
			return domain.Task{}, fmt.Errorf("%w: invalid UUID in task_id: %w", domain.ErrFatal, err)
		}
		taskID = parsedUUID
	}

	taskType := payload.Type
	if taskType == "" {
		taskType = msg.Metadata.Type
	}
	if taskType == "" {
		taskType = "default.task"
	}

	return domain.NewTask(taskID, taskType, payload.Content)
}

func (uc *ProcessTaskUseCase) persistAndComplete(
	ctx context.Context,
	task *domain.Task,
	msgID string,
	logger *slog.Logger,
) error {
	if uc.repo != nil {
		createdTask, err := uc.repo.Create(ctx, *task)
		if err != nil {
			if errors.Is(err, domain.ErrTaskAlreadyExists) {
				logger.WarnContext(ctx, "task already exists in repository; treating as replay")
			} else {
				logger.ErrorContext(ctx, "transient error writing task to database", "error", err)
				if uc.idempotencyStore != nil {
					_ = uc.idempotencyStore.MarkFailed(ctx, msgID)
				}
				return fmt.Errorf("%w: failed to persist task: %w", domain.ErrTransient, err)
			}
		} else {
			*task = createdTask
		}

		task.MarkCompleted(fmt.Sprintf("processed %s task successfully", task.Type))
		if _, updateErr := uc.repo.Update(ctx, *task); updateErr != nil {
			logger.WarnContext(ctx, "failed to update task completion status", "error", updateErr)
		}
	}

	if uc.idempotencyStore != nil {
		if err := uc.idempotencyStore.MarkCompleted(ctx, msgID); err != nil {
			logger.WarnContext(ctx, "failed to mark message completed in idempotency store", "error", err)
		}
	}

	return nil
}

func (uc *ProcessTaskUseCase) handleFatal(
	ctx context.Context,
	msg domain.EventMessage,
	msgID string,
	fatalErr error,
) error {
	if uc.dlqPublisher != nil {
		if pubErr := uc.dlqPublisher.PublishToDLQ(ctx, msg, fatalErr); pubErr != nil {
			uc.logger.ErrorContext(ctx, "failed to route poison pill to DLQ", "error", pubErr)
		}
	}
	if uc.idempotencyStore != nil {
		_ = uc.idempotencyStore.MarkCompleted(ctx, msgID)
	}
	return nil
}
