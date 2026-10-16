-- +goose Up
CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY,
    type VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    result TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks (type);

CREATE TABLE IF NOT EXISTS processed_messages (
    message_id VARCHAR(255) PRIMARY KEY,
    consumer_group VARCHAR(100) NOT NULL DEFAULT 'default',
    status VARCHAR(50) NOT NULL DEFAULT 'COMPLETED',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_processed_messages_group ON processed_messages (consumer_group);

-- +goose Down
DROP TABLE IF EXISTS processed_messages;
DROP TABLE IF EXISTS tasks;
