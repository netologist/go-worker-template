package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/gen/sqlc"
)

// ToDomainTask converts a sqlc.Task database record to a domain.Task entity.
func ToDomainTask(row sqlc.Task) domain.Task {
	var processedAt *time.Time
	if row.ProcessedAt.Valid {
		t := row.ProcessedAt.Time
		processedAt = &t
	}

	return domain.Task{
		ID:          row.ID,
		Type:        row.Type,
		Payload:     row.Payload,
		Status:      domain.TaskStatus(row.Status),
		Result:      row.Result,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		ProcessedAt: processedAt,
	}
}

// ToDomainTaskList converts a slice of sqlc.Task database records to domain.Task entities.
func ToDomainTaskList(rows []sqlc.Task) []domain.Task {
	if rows == nil {
		return nil
	}
	items := make([]domain.Task, len(rows))
	for i, r := range rows {
		items[i] = ToDomainTask(r)
	}
	return items
}

// ToPgTimestamptz converts a nullable *time.Time pointer into pgtype.Timestamptz.
func ToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
