package domain

import "time"

// MessageMetadata holds standardized metadata for asynchronous event messages.
type MessageMetadata struct {
	ID            string            `json:"id"`
	CorrelationID string            `json:"correlation_id"`
	TraceID       string            `json:"trace_id"`
	Source        string            `json:"source"`
	Type          string            `json:"type"`
	Timestamp     time.Time         `json:"timestamp"`
	AttemptCount  int               `json:"attempt_count"`
	Headers       map[string]string `json:"headers"`
}

// EventMessage represents a raw event message consumed from an asynchronous messaging broker.
type EventMessage struct {
	Metadata MessageMetadata `json:"metadata"`
	Payload  []byte          `json:"payload"`
}
