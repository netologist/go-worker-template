package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxTaskTypeLen = 100

// TaskStatus represents the lifecycle state of a background task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "PENDING"
	TaskStatusProcessing TaskStatus = "PROCESSING"
	TaskStatusCompleted  TaskStatus = "COMPLETED"
	TaskStatusFailed     TaskStatus = "FAILED"
)

// Task represents the core business entity processed by the worker service.
type Task struct {
	ID          uuid.UUID  `json:"id"`
	Type        string     `json:"type"`
	Payload     string     `json:"payload"`
	Status      TaskStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// NewTask constructs a new valid Task entity, enforcing domain invariants.
func NewTask(id uuid.UUID, taskType string, payload string) (Task, error) {
	if id == uuid.Nil {
		return Task{}, fmt.Errorf("%w: task ID cannot be nil UUID", ErrInvalidTask)
	}

	trimmedType := strings.TrimSpace(taskType)
	if trimmedType == "" {
		return Task{}, fmt.Errorf("%w: task type cannot be empty", ErrInvalidTask)
	}
	if len(trimmedType) > maxTaskTypeLen {
		return Task{}, fmt.Errorf("%w: task type exceeds maximum length of %d characters",
			ErrInvalidTask, maxTaskTypeLen)
	}

	now := time.Now().UTC()
	return Task{
		ID:        id,
		Type:      trimmedType,
		Payload:   payload,
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// MarkCompleted transitions task status to COMPLETED and sets completion time.
func (t *Task) MarkCompleted(result string) {
	now := time.Now().UTC()
	t.Status = TaskStatusCompleted
	t.Result = result
	t.UpdatedAt = now
	t.ProcessedAt = &now
}

// MarkFailed transitions task status to FAILED and records the error message.
func (t *Task) MarkFailed(failureReason string) {
	now := time.Now().UTC()
	t.Status = TaskStatusFailed
	t.Result = failureReason
	t.UpdatedAt = now
	t.ProcessedAt = &now
}
