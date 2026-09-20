-- +goose Up
-- Частичный индекс для воркера отмен
CREATE INDEX idx_bookings_cancel_command_sent_at ON bookings(cancel_command_sent_at) WHERE status = 'cancellation_pending';

-- Индекс для запросов статистики по датам
CREATE INDEX idx_bookings_created_at ON bookings(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_cancel_command_sent_at;
DROP INDEX IF EXISTS idx_bookings_created_at;
