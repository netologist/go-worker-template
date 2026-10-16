package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

const (
	statusIdempCompleted = "COMPLETED"
	statusProc           = "PROCESSING"
)

func TestIdempotencyStore(t *testing.T) {
	ctx := context.Background()
	msgID := "msg-12345"

	t.Run("try acquire success on first arrival", func(t *testing.T) {
		q := &mockQuerier{}
		store := NewIdempotencyStoreWithQuerier(q, "group-a")

		q.On("TryAcquireMessage", ctx, mock.MatchedBy(func(arg sqlc.TryAcquireMessageParams) bool {
			return arg.MessageID == msgID && arg.ConsumerGroup == "group-a"
		})).Return(sqlc.ProcessedMessage{
			MessageID: msgID,
			Status:    statusProc,
		}, nil)

		acquired, err := store.TryAcquire(ctx, msgID, time.Hour)
		require.NoError(t, err)
		assert.True(t, acquired)
		q.AssertExpectations(t)
	})

	t.Run("try acquire returns false when already completed", func(t *testing.T) {
		q := &mockQuerier{}
		store := NewIdempotencyStoreWithQuerier(q, "group-a")

		q.On("TryAcquireMessage", ctx, mock.Anything).Return(sqlc.ProcessedMessage{}, pgx.ErrNoRows)
		q.On("GetMessageStatus", ctx, msgID).Return(sqlc.ProcessedMessage{
			MessageID:   msgID,
			Status:      statusIdempCompleted,
			ProcessedAt: time.Now().UTC(),
		}, nil)

		acquired, err := store.TryAcquire(ctx, msgID, time.Hour)
		require.NoError(t, err)
		assert.False(t, acquired)
	})

	t.Run("mark completed", func(t *testing.T) {
		q := &mockQuerier{}
		store := NewIdempotencyStoreWithQuerier(q, "group-a")

		q.On("MarkMessageCompleted", ctx, mock.MatchedBy(func(arg sqlc.MarkMessageCompletedParams) bool {
			return arg.MessageID == msgID
		})).Return(nil)

		err := store.MarkCompleted(ctx, msgID)
		require.NoError(t, err)
	})

	t.Run("mark failed deletes key", func(t *testing.T) {
		q := &mockQuerier{}
		store := NewIdempotencyStoreWithQuerier(q, "group-a")

		q.On("MarkMessageFailed", ctx, msgID).Return(nil)

		err := store.MarkFailed(ctx, msgID)
		require.NoError(t, err)
	})
}
