-- +goose Up
CREATE TABLE booking_history (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    previous_status VARCHAR(50),
    new_status VARCHAR(50) NOT NULL,
    initiator VARCHAR(255) NOT NULL,
    reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Индекс для быстрого поиска истории конкретного бронирования с сортировкой
CREATE INDEX idx_booking_history_booking_id_created_at ON booking_history(booking_id, created_at DESC);

-- +goose Down
DROP TABLE booking_history;