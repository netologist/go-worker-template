-- name: TryAcquireMessage :one
INSERT INTO processed_messages (
    message_id, consumer_group, status, processed_at
) VALUES (
    $1, $2, 'PROCESSING', $3
)
ON CONFLICT (message_id) DO NOTHING
RETURNING *;

-- name: MarkMessageCompleted :exec
UPDATE processed_messages
SET status = 'COMPLETED',
    processed_at = $2
WHERE message_id = $1;

-- name: MarkMessageFailed :exec
DELETE FROM processed_messages
WHERE message_id = $1;

-- name: GetMessageStatus :one
SELECT * FROM processed_messages
WHERE message_id = $1 LIMIT 1;
