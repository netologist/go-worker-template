package domain

import "errors"

// ErrTransient represents a temporary, recoverable failure (e.g., database deadlock, network timeout).
// When encountered, the message should be retried or NACKed with exponential backoff.
var ErrTransient = errors.New("transient error: operation should be retried")

// ErrFatal represents a non-recoverable, permanent failure (e.g., malformed payload, poison pill, schema violation).
// When encountered, the message should not be retried and should be routed directly to the Dead Letter Queue (DLQ).
var ErrFatal = errors.New("fatal error: poison pill or permanent failure")

// ErrInvalidTask indicates that a task violates domain invariants.
var ErrInvalidTask = errors.New("invalid task invariants")

// ErrTaskNotFound indicates the requested task does not exist.
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskAlreadyExists indicates a task with the given ID already exists.
var ErrTaskAlreadyExists = errors.New("task already exists")
