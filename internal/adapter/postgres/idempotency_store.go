package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

type idempotencyStore struct {
	queries       sqlc.Querier
	consumerGroup string
}

var _ domain.IdempotencyStore = (*idempotencyStore)(nil)

// NewIdempotencyStore creates a new Postgres-backed idempotency store.
func NewIdempotencyStore(pool *pgxpool.Pool, consumerGroup string) *idempotencyStore {
	return NewIdempotencyStoreWithQuerier(sqlc.New(pool), consumerGroup)
}

// NewIdempotencyStoreWithQuerier creates a new Postgres-backed idempotency store with an injected sqlc.Querier.
func NewIdempotencyStoreWithQuerier(queries sqlc.Querier, consumerGroup string) *idempotencyStore {
	if consumerGroup == "" {
		consumerGroup = "default"
	}
	return &idempotencyStore{
		queries:       queries,
		consumerGroup: consumerGroup,
	}
}

func (s *idempotencyStore) TryAcquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	_, err := s.queries.TryAcquireMessage(ctx, sqlc.TryAcquireMessageParams{
		MessageID:     key,
		ConsumerGroup: s.consumerGroup,
		ProcessedAt:   now,
	})
	if err == nil {
		// Insert succeeded: lock/claim acquired
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Key already exists in processed_messages. Inspect existing status and TTL.
		msg, getErr := s.queries.GetMessageStatus(ctx, key)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return false, nil
			}
			return false, getErr
		}

		if msg.Status == "PROCESSING" && ttl > 0 && now.Sub(msg.ProcessedAt) > ttl {
			// In-flight processing expired (node crash / hung worker). Allow re-acquisition.
			return true, nil
		}

		// Already completed or actively processing within TTL.
		return false, nil
	}

	return false, err
}

func (s *idempotencyStore) MarkCompleted(ctx context.Context, key string) error {
	return s.queries.MarkMessageCompleted(ctx, sqlc.MarkMessageCompletedParams{
		MessageID:   key,
		ProcessedAt: time.Now().UTC(),
	})
}

func (s *idempotencyStore) MarkFailed(ctx context.Context, key string) error {
	return s.queries.MarkMessageFailed(ctx, key)
}
