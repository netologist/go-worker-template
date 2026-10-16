package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/domain"
	domainmocks "github.com/netologist/go-worker-template/internal/gen/mocks/domain"
	"github.com/netologist/go-worker-template/internal/usecase"
)

const taskTypeEmail = "email.send"

func TestProcessTaskUseCase_Success(t *testing.T) {
	ctx := context.Background()
	repoMock := domainmocks.NewMockTaskRepository(t)
	idempMock := domainmocks.NewMockIdempotencyStore(t)
	dlqMock := domainmocks.NewMockDeadLetterPublisher(t)

	msgID := "msg-123"
	taskID := uuid.New()
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{
			ID:      msgID,
			Type:    taskTypeEmail,
			TraceID: "trace-xyz",
		},
		Payload: []byte(`{"task_id":"` + taskID.String() + `","content":"send welcome email"}`),
	}

	idempMock.EXPECT().
		TryAcquire(ctx, msgID, mock.AnythingOfType("time.Duration")).
		Return(true, nil).
		Once()

	repoMock.EXPECT().
		Create(ctx, mock.MatchedBy(func(task domain.Task) bool {
			return task.ID == taskID && task.Type == taskTypeEmail
		})).
		Return(domain.Task{ID: taskID, Type: taskTypeEmail, Status: domain.TaskStatusPending}, nil).
		Once()

	repoMock.EXPECT().
		Update(ctx, mock.MatchedBy(func(task domain.Task) bool {
			return task.ID == taskID && task.Status == domain.TaskStatusCompleted
		})).
		Return(domain.Task{ID: taskID, Status: domain.TaskStatusCompleted}, nil).
		Once()

	idempMock.EXPECT().
		MarkCompleted(ctx, msgID).
		Return(nil).
		Once()

	uc := usecase.NewProcessTaskUseCase(repoMock, idempMock, dlqMock, 24*time.Hour, nil)
	err := uc.Process(ctx, msg)
	require.NoError(t, err)
}

func TestProcessTaskUseCase_IdempotencyHit(t *testing.T) {
	ctx := context.Background()
	repoMock := domainmocks.NewMockTaskRepository(t)
	idempMock := domainmocks.NewMockIdempotencyStore(t)
	dlqMock := domainmocks.NewMockDeadLetterPublisher(t)

	msgID := "msg-duplicate"
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{ID: msgID, Type: taskTypeEmail},
		Payload:  []byte(`{"content":"duplicate payload"}`),
	}

	idempMock.EXPECT().
		TryAcquire(ctx, msgID, mock.AnythingOfType("time.Duration")).
		Return(false, nil).
		Once()

	uc := usecase.NewProcessTaskUseCase(repoMock, idempMock, dlqMock, 24*time.Hour, nil)
	err := uc.Process(ctx, msg)
	require.NoError(t, err)
}

func TestProcessTaskUseCase_TransientIdempotencyError(t *testing.T) {
	ctx := context.Background()
	repoMock := domainmocks.NewMockTaskRepository(t)
	idempMock := domainmocks.NewMockIdempotencyStore(t)
	dlqMock := domainmocks.NewMockDeadLetterPublisher(t)

	msgID := "msg-db-error"
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{ID: msgID, Type: taskTypeEmail},
		Payload:  []byte(`{"content":"test"}`),
	}

	idempMock.EXPECT().
		TryAcquire(ctx, msgID, mock.AnythingOfType("time.Duration")).
		Return(false, errors.New("connection reset by peer")).
		Once()

	uc := usecase.NewProcessTaskUseCase(repoMock, idempMock, dlqMock, 24*time.Hour, nil)
	err := uc.Process(ctx, msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTransient)
}

func TestProcessTaskUseCase_PoisonPill(t *testing.T) {
	ctx := context.Background()
	repoMock := domainmocks.NewMockTaskRepository(t)
	idempMock := domainmocks.NewMockIdempotencyStore(t)
	dlqMock := domainmocks.NewMockDeadLetterPublisher(t)

	msgID := "msg-poison"
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{ID: msgID, Type: taskTypeEmail},
		Payload:  []byte(`{malformed json`),
	}

	idempMock.EXPECT().
		TryAcquire(ctx, msgID, mock.AnythingOfType("time.Duration")).
		Return(true, nil).
		Once()

	dlqMock.EXPECT().
		PublishToDLQ(ctx, msg, mock.MatchedBy(func(err error) bool {
			return errors.Is(err, domain.ErrFatal)
		})).
		Return(nil).
		Once()

	idempMock.EXPECT().
		MarkCompleted(ctx, msgID).
		Return(nil).
		Once()

	uc := usecase.NewProcessTaskUseCase(repoMock, idempMock, dlqMock, 24*time.Hour, nil)
	err := uc.Process(ctx, msg)
	require.NoError(t, err)
}

func TestProcessTaskUseCase_TransientRepoError(t *testing.T) {
	ctx := context.Background()
	repoMock := domainmocks.NewMockTaskRepository(t)
	idempMock := domainmocks.NewMockIdempotencyStore(t)
	dlqMock := domainmocks.NewMockDeadLetterPublisher(t)

	msgID := "msg-db-fail"
	msg := domain.EventMessage{
		Metadata: domain.MessageMetadata{ID: msgID, Type: "order.process"},
		Payload:  []byte(`{"content":"order payload"}`),
	}

	idempMock.EXPECT().
		TryAcquire(ctx, msgID, mock.AnythingOfType("time.Duration")).
		Return(true, nil).
		Once()

	repoMock.EXPECT().
		Create(ctx, mock.Anything).
		Return(domain.Task{}, errors.New("db deadlock")).
		Once()

	idempMock.EXPECT().
		MarkFailed(ctx, msgID).
		Return(nil).
		Once()

	uc := usecase.NewProcessTaskUseCase(repoMock, idempMock, dlqMock, 24*time.Hour, nil)
	err := uc.Process(ctx, msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTransient)
}
