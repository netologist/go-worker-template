package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netologist/go-worker-template/internal/adapter/consumer"
	"github.com/netologist/go-worker-template/internal/adapter/postgres"
	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

// Listener implements consumer.MessageListener via Postgres SKIP LOCKED polling.
type Listener struct {
	pool         *pgxpool.Pool
	queries      sqlc.Querier
	concurrency  int
	pollInterval time.Duration
	batchSize    int32
	logger       *slog.Logger

	mu        sync.Mutex
	cancel    context.CancelFunc
	drainDone chan struct{}
}

// Config configures the OutboxListener.
type Config struct {
	Concurrency  int
	PollInterval time.Duration
	BatchSize    int32
	Logger       *slog.Logger
}

// NewListener creates a new Postgres Outbox listener.
func NewListener(pool *pgxpool.Pool, cfg Config) *Listener {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Listener{
		pool:         pool,
		queries:      sqlc.New(pool),
		concurrency:  cfg.Concurrency,
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		logger:       cfg.Logger,
		drainDone:    make(chan struct{}),
	}
}

// Start begins the polling loop until ctx is canceled or Stop() is called.
func (l *Listener) Start(ctx context.Context, processor domain.TaskProcessor) error {
	runCtx, cancel := context.WithCancel(ctx)
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()

	defer close(l.drainDone)

	l.logger.InfoContext(runCtx, "starting postgres outbox consumer loop",
		"concurrency", l.concurrency,
		"poll_interval", l.pollInterval,
		"batch_size", l.batchSize,
	)

	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()

	sem := make(chan struct{}, l.concurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-runCtx.Done():
			l.logger.Info("outbox poll loop canceled; draining in-flight workers")
			wg.Wait()
			return nil

		case <-ticker.C:
			l.pollBatch(runCtx, processor, sem, &wg)
		}
	}
}

func (l *Listener) pollBatch(
	ctx context.Context,
	processor domain.TaskProcessor,
	sem chan struct{},
	wg *sync.WaitGroup,
) {
	tasks, err := l.queries.FetchPendingTasksForUpdate(ctx, l.batchSize)
	if err != nil {
		if ctx.Err() == nil {
			l.logger.ErrorContext(ctx, "failed to query pending tasks from outbox", "error", err)
		}
		return
	}

	for _, t := range tasks {
		sem <- struct{}{}
		wg.Add(1)

		taskRecord := t
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			l.dispatchRecord(ctx, processor, taskRecord)
		}()
	}
}

func (l *Listener) dispatchRecord(ctx context.Context, processor domain.TaskProcessor, taskRecord sqlc.Task) {
	taskDomain := postgres.ToDomainTask(taskRecord)
	payloadBytes, _ := json.Marshal(map[string]string{
		"task_id": taskDomain.ID.String(),
		"type":    taskDomain.Type,
		"content": taskDomain.Payload,
	})

	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{
			ID:        taskDomain.ID.String(),
			Type:      taskDomain.Type,
			Timestamp: taskDomain.CreatedAt,
		},
		Payload: payloadBytes,
	}

	if procErr := consumer.SafeProcess(ctx, processor, msg, l.logger); procErr != nil {
		l.logger.ErrorContext(ctx, "error processing outbox task",
			"task_id", taskDomain.ID,
			"error", procErr,
		)
	}
}

// Stop gracefully stops the consumer loop.
func (l *Listener) Stop(ctx context.Context) error {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	select {
	case <-l.drainDone:
		l.logger.Info("outbox listener successfully drained and stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for outbox listener to drain: %w", ctx.Err())
	}
}
