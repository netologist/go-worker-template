package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TaskProcessor is the primary inbound port representing the consumer execution contract.
type TaskProcessor interface {
	Process(ctx context.Context, msg EventMessage) error
}

// TaskRepository is an outbound port defining persistent storage operations for tasks.
type TaskRepository interface {
	Create(ctx context.Context, task Task) (Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (Task, error)
	Update(ctx context.Context, task Task) (Task, error)
}

// IdempotencyStore is an outbound port defining atomic message deduplication semantics.
type IdempotencyStore interface {
	// TryAcquire atomically attempts to lock or claim message execution for key.
	// Returns true if acquired (first time processing), false if already processed or in-flight.
	TryAcquire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// MarkCompleted flags the message key as definitively processed.
	MarkCompleted(ctx context.Context, key string) error
	// MarkFailed marks the key as failed or releases lock so it can be retried if transient.
	MarkFailed(ctx context.Context, key string) error
}

// DeadLetterPublisher is an outbound port for forwarding poison pills or permanently failed messages to a DLQ.
type DeadLetterPublisher interface {
	PublishToDLQ(ctx context.Context, msg EventMessage, reason error) error
}
