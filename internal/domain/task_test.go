package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/domain"
)

func TestNewTask_Success(t *testing.T) {
	id := uuid.New()
	task, err := domain.NewTask(id, "email.send", `{"to":"user@example.com"}`)
	require.NoError(t, err)

	assert.Equal(t, id, task.ID)
	assert.Equal(t, "email.send", task.Type)
	assert.JSONEq(t, `{"to":"user@example.com"}`, task.Payload)
	assert.Equal(t, domain.TaskStatusPending, task.Status)
	assert.False(t, task.CreatedAt.IsZero())
	assert.False(t, task.UpdatedAt.IsZero())
	assert.Nil(t, task.ProcessedAt)
}

func TestNewTask_Invariants(t *testing.T) {
	t.Run("nil uuid fails", func(t *testing.T) {
		_, err := domain.NewTask(uuid.Nil, "test", "payload")
		require.ErrorIs(t, err, domain.ErrInvalidTask)
	})

	t.Run("empty task type fails", func(t *testing.T) {
		_, err := domain.NewTask(uuid.New(), "   ", "payload")
		require.ErrorIs(t, err, domain.ErrInvalidTask)
	})

	t.Run("overlong task type fails", func(t *testing.T) {
		overlong := string(make([]byte, 101))
		_, err := domain.NewTask(uuid.New(), overlong, "payload")
		require.ErrorIs(t, err, domain.ErrInvalidTask)
	})
}

func TestTask_StatusTransitions(t *testing.T) {
	id := uuid.New()
	task, err := domain.NewTask(id, "report.generate", "{}")
	require.NoError(t, err)

	t.Run("mark completed", func(t *testing.T) {
		task.MarkCompleted("success: report-123.pdf")
		assert.Equal(t, domain.TaskStatusCompleted, task.Status)
		assert.Equal(t, "success: report-123.pdf", task.Result)
		assert.NotNil(t, task.ProcessedAt)
	})

	t.Run("mark failed", func(t *testing.T) {
		task.MarkFailed("error: timeout")
		assert.Equal(t, domain.TaskStatusFailed, task.Status)
		assert.Equal(t, "error: timeout", task.Result)
	})
}
