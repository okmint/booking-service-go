-- +goose Up
ALTER TABLE bookings
    ADD COLUMN previous_status VARCHAR(30) DEFAULT NULL,
    ADD COLUMN cancel_command_sent_at TIMESTAMPTZ DEFAULT NULL;

-- +goose Down
ALTER TABLE bookings
    DROP COLUMN IF EXISTS previous_status,
    DROP COLUMN IF EXISTS cancel_command_sent_at;
