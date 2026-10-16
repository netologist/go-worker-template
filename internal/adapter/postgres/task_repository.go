package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

type taskRepository struct {
	queries sqlc.Querier
}

var _ domain.TaskRepository = (*taskRepository)(nil)

// NewTaskRepository creates a new task repository with a Postgres connection pool.
func NewTaskRepository(pool *pgxpool.Pool) *taskRepository {
	return NewTaskRepositoryWithQuerier(sqlc.New(pool))
}

// NewTaskRepositoryWithQuerier creates a new task repository with an injected sqlc.Querier.
func NewTaskRepositoryWithQuerier(queries sqlc.Querier) *taskRepository {
	return &taskRepository{
		queries: queries,
	}
}

func (r *taskRepository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	row, err := r.queries.CreateTask(ctx, sqlc.CreateTaskParams{
		ID:          task.ID,
		Type:        task.Type,
		Payload:     task.Payload,
		Status:      string(task.Status),
		Result:      task.Result,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		ProcessedAt: ToPgTimestamptz(task.ProcessedAt),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Task{}, fmt.Errorf("%w: %w", domain.ErrTaskAlreadyExists, err)
		}
		return domain.Task{}, err
	}
	return ToDomainTask(row), nil
}

func (r *taskRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Task, error) {
	row, err := r.queries.GetTaskByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Task{}, domain.ErrTaskNotFound
		}
		return domain.Task{}, err
	}
	return ToDomainTask(row), nil
}

func (r *taskRepository) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	row, err := r.queries.UpdateTask(ctx, sqlc.UpdateTaskParams{
		ID:          task.ID,
		Type:        task.Type,
		Payload:     task.Payload,
		Status:      string(task.Status),
		Result:      task.Result,
		UpdatedAt:   task.UpdatedAt,
		ProcessedAt: ToPgTimestamptz(task.ProcessedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Task{}, domain.ErrTaskNotFound
		}
		return domain.Task{}, err
	}
	return ToDomainTask(row), nil
}

func (r *taskRepository) FetchPending(ctx context.Context, limit int32) ([]domain.Task, error) {
	rows, err := r.queries.FetchPendingTasksForUpdate(ctx, limit)
	if err != nil {
		return nil, err
	}
	return ToDomainTaskList(rows), nil
}
