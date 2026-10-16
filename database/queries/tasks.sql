-- name: CreateTask :one
INSERT INTO tasks (
    id, type, payload, status, result, created_at, updated_at, processed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks
WHERE id = $1 LIMIT 1;

-- name: UpdateTask :one
UPDATE tasks
SET type = $2,
    payload = $3,
    status = $4,
    result = $5,
    updated_at = $6,
    processed_at = $7
WHERE id = $1
RETURNING *;

-- name: FetchPendingTasksForUpdate :many
SELECT * FROM tasks
WHERE status = 'PENDING'
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;
