package dlq_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/adapter/dlq"
	"github.com/netologist/go-worker-template/internal/domain"
)

func TestDLQPublishers(t *testing.T) {
	ctx := context.Background()
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{ID: "msg-dlq-1", Type: "order.process"},
		Payload:  []byte(`{"bad":"payload"}`),
	}
	reason := errors.New("poison pill format")

	t.Run("log publisher does not fail", func(t *testing.T) {
		lp := dlq.NewLogPublisher(nil)
		err := lp.PublishToDLQ(ctx, msg, reason)
		require.NoError(t, err)
	})

	t.Run("in memory publisher records message and count", func(t *testing.T) {
		mp := dlq.NewInMemoryPublisher()
		require.Equal(t, 0, mp.Count())

		err := mp.PublishToDLQ(ctx, msg, reason)
		require.NoError(t, err)

		assert.Equal(t, 1, mp.Count())
		assert.Equal(t, "msg-dlq-1", mp.Messages[0].Metadata.ID)
		assert.Equal(t, reason, mp.Reasons[0])
	})
}
