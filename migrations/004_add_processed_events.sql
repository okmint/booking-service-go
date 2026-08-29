-- +goose Up
CREATE TABLE processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE processed_events;