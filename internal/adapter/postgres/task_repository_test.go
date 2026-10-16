package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

const (
	taskTypeEmail   = "email.send"
	statusCompleted = "COMPLETED"
)

type mockQuerier struct {
	mock.Mock
}

var _ sqlc.Querier = (*mockQuerier)(nil)

func (m *mockQuerier) CreateTask(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.Task, error) {
	args := m.Called(ctx, arg)
	if row, ok := args.Get(0).(sqlc.Task); ok {
		return row, args.Error(1)
	}
	return sqlc.Task{}, args.Error(1)
}

func (m *mockQuerier) GetTaskByID(ctx context.Context, id uuid.UUID) (sqlc.Task, error) {
	args := m.Called(ctx, id)
	if row, ok := args.Get(0).(sqlc.Task); ok {
		return row, args.Error(1)
	}
	return sqlc.Task{}, args.Error(1)
}

func (m *mockQuerier) UpdateTask(ctx context.Context, arg sqlc.UpdateTaskParams) (sqlc.Task, error) {
	args := m.Called(ctx, arg)
	if row, ok := args.Get(0).(sqlc.Task); ok {
		return row, args.Error(1)
	}
	return sqlc.Task{}, args.Error(1)
}

func (m *mockQuerier) FetchPendingTasksForUpdate(ctx context.Context, limit int32) ([]sqlc.Task, error) {
	args := m.Called(ctx, limit)
	if rows, ok := args.Get(0).([]sqlc.Task); ok {
		return rows, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockQuerier) TryAcquireMessage(
	ctx context.Context,
	arg sqlc.TryAcquireMessageParams,
) (sqlc.ProcessedMessage, error) {
	args := m.Called(ctx, arg)
	if row, ok := args.Get(0).(sqlc.ProcessedMessage); ok {
		return row, args.Error(1)
	}
	return sqlc.ProcessedMessage{}, args.Error(1)
}

func (m *mockQuerier) GetMessageStatus(ctx context.Context, messageID string) (sqlc.ProcessedMessage, error) {
	args := m.Called(ctx, messageID)
	if row, ok := args.Get(0).(sqlc.ProcessedMessage); ok {
		return row, args.Error(1)
	}
	return sqlc.ProcessedMessage{}, args.Error(1)
}

func (m *mockQuerier) MarkMessageCompleted(ctx context.Context, arg sqlc.MarkMessageCompletedParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) MarkMessageFailed(ctx context.Context, messageID string) error {
	args := m.Called(ctx, messageID)
	return args.Error(0)
}

func TestTaskRepository_Create(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	now := time.Now().UTC()

	t.Run("create task success", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		input := domain.Task{
			ID:        taskID,
			Type:      taskTypeEmail,
			Payload:   `{"to":"a@b.com"}`,
			Status:    domain.TaskStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		}

		q.On("CreateTask", ctx, mock.Anything).Return(sqlc.Task{
			ID:          taskID,
			Type:        taskTypeEmail,
			Payload:     `{"to":"a@b.com"}`,
			Status:      "PENDING",
			CreatedAt:   now,
			UpdatedAt:   now,
			ProcessedAt: ToPgTimestamptz(nil),
		}, nil)

		created, err := repo.Create(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, taskID, created.ID)
		assert.Equal(t, domain.TaskStatusPending, created.Status)
		q.AssertExpectations(t)
	})

	t.Run("create task unique violation", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		input := domain.Task{ID: taskID, Type: taskTypeEmail}
		q.On("CreateTask", ctx, mock.Anything).Return(sqlc.Task{}, &pgconn.PgError{Code: "23505"})

		_, err := repo.Create(ctx, input)
		require.ErrorIs(t, err, domain.ErrTaskAlreadyExists)
	})
}

func TestTaskRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()

	t.Run("get by id success", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		q.On("GetTaskByID", ctx, taskID).Return(sqlc.Task{
			ID:     taskID,
			Type:   taskTypeEmail,
			Status: statusCompleted,
		}, nil)

		task, err := repo.GetByID(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCompleted, task.Status)
	})

	t.Run("get by id not found", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		q.On("GetTaskByID", ctx, taskID).Return(sqlc.Task{}, pgx.ErrNoRows)

		_, err := repo.GetByID(ctx, taskID)
		require.ErrorIs(t, err, domain.ErrTaskNotFound)
	})
}

func TestTaskRepository_UpdateAndFetch(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()

	t.Run("update task success", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		input := domain.Task{
			ID:     taskID,
			Type:   taskTypeEmail,
			Status: domain.TaskStatusCompleted,
		}

		q.On("UpdateTask", ctx, mock.Anything).Return(sqlc.Task{
			ID:     taskID,
			Type:   taskTypeEmail,
			Status: statusCompleted,
		}, nil)

		updated, err := repo.Update(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCompleted, updated.Status)
	})

	t.Run("fetch pending tasks", func(t *testing.T) {
		q := &mockQuerier{}
		repo := NewTaskRepositoryWithQuerier(q)

		q.On("FetchPendingTasksForUpdate", ctx, int32(10)).Return([]sqlc.Task{
			{ID: taskID, Type: taskTypeEmail, Status: "PENDING"},
		}, nil)

		tasks, err := repo.FetchPending(ctx, 10)
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
	})
}
