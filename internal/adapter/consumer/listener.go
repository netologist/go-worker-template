package consumer

import (
	"context"

	"github.com/netologist/go-worker-template/internal/domain"
)

// MessageListener defines the pluggable inbound consumer contract for message brokers.
// Concrete implementations (RabbitMQ, Kafka, SQS, Postgres Outbox) must implement this interface.
type MessageListener interface {
	// Start begins consuming messages in a blocking or background loop, dispatching to processor.
	Start(ctx context.Context, processor domain.TaskProcessor) error
	// Stop gracefully halts polling and waits for in-flight tasks to drain.
	Stop(ctx context.Context) error
}
