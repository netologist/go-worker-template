package consumer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/adapter/consumer"
	"github.com/netologist/go-worker-template/internal/domain"
)

type panicProcessor struct{}

func (p *panicProcessor) Process(ctx context.Context, msg domain.EventMessage) error {
	panic("unexpected nil pointer dereference")
}

type successProcessor struct{}

func (s *successProcessor) Process(ctx context.Context, msg domain.EventMessage) error {
	return nil
}

func TestSafeProcess(t *testing.T) {
	ctx := context.Background()
	msg := domain.EventMessage{Metadata: domain.MessageMetadata{ID: "msg-test"}}

	t.Run("handles normal execution", func(t *testing.T) {
		err := consumer.SafeProcess(ctx, &successProcessor{}, msg, nil)
		require.NoError(t, err)
	})

	t.Run("recovers from panic and returns error", func(t *testing.T) {
		err := consumer.SafeProcess(ctx, &panicProcessor{}, msg, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered during task processing")
	})
}
