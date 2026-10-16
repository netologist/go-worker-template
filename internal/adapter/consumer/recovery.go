package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/netologist/go-worker-template/internal/domain"
)

// SafeProcess wraps processor.Process with panic recovery to ensure worker goroutines do not crash the runtime.
func SafeProcess(
	ctx context.Context,
	processor domain.TaskProcessor,
	msg domain.EventMessage,
	logger *slog.Logger,
) (err error) {
	if logger == nil {
		logger = slog.Default()
	}

	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			err = fmt.Errorf("panic recovered during task processing: %v", r)
			logger.ErrorContext(ctx, "panic recovered in worker process loop",
				"error", err,
				"panic_value", r,
				"message_id", msg.Metadata.ID,
				"stack", stack,
			)
		}
	}()

	return processor.Process(ctx, msg)
}
