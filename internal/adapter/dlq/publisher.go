package dlq

import (
	"context"
	"log/slog"
	"sync"

	"github.com/netologist/go-worker-template/internal/domain"
)

// LogPublisher implements domain.DeadLetterPublisher by recording poison pills to structured audit logs.
type LogPublisher struct {
	logger *slog.Logger
}

var _ domain.DeadLetterPublisher = (*LogPublisher)(nil)

// NewLogPublisher creates a new LogPublisher.
func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogPublisher{logger: logger}
}

// PublishToDLQ logs dead-lettered messages.
func (p *LogPublisher) PublishToDLQ(ctx context.Context, msg domain.EventMessage, reason error) error {
	p.logger.ErrorContext(ctx, "message routed to dead-letter storage",
		"message_id", msg.Metadata.ID,
		"correlation_id", msg.Metadata.CorrelationID,
		"type", msg.Metadata.Type,
		"error_reason", reason.Error(),
		"payload_size", len(msg.Payload),
	)
	return nil
}

// InMemoryPublisher is a thread-safe in-memory DLQ store primarily used for testing and validation.
type InMemoryPublisher struct {
	mu       sync.Mutex
	Messages []domain.EventMessage
	Reasons  []error
}

var _ domain.DeadLetterPublisher = (*InMemoryPublisher)(nil)

// NewInMemoryPublisher initializes a new InMemoryPublisher.
func NewInMemoryPublisher() *InMemoryPublisher {
	return &InMemoryPublisher{
		Messages: make([]domain.EventMessage, 0),
		Reasons:  make([]error, 0),
	}
}

// PublishToDLQ records the dead-lettered message in memory.
func (p *InMemoryPublisher) PublishToDLQ(_ context.Context, msg domain.EventMessage, reason error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Messages = append(p.Messages, msg)
	p.Reasons = append(p.Reasons, reason)
	return nil
}

// Count returns the number of captured DLQ messages.
func (p *InMemoryPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Messages)
}
