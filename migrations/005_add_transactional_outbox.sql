-- +goose Up
CREATE TABLE outbox_messages (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'pending' NOT NULL,
    retry_count INT DEFAULT 0 NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE
);

-- Индекс для быстрого поллинга необработанных сообщений воркером
CREATE INDEX idx_outbox_messages_status_created_at ON outbox_messages(status, created_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE outbox_messages;