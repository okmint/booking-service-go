package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// CancellationWorker -- фоновый воркер для обработки зависших отмен.
//
// Логика работы:
//  1. Найти бронирования в статусе cancellation_pending, где cancel_command_sent_at старше таймаута.
//  2. Для каждого такого бронирования повторно отправить команду в Catalog через RabbitMQ.
//  3. Статус в БД не меняется, полагаемся на ответ от Catalog или DLQ rollback
type CancellationWorker struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	interval  time.Duration
	timeout   time.Duration
	batchSize int
	logger    *zap.Logger
}

// NewCancellationWorker создаёт новый воркер зависших отмен.
func NewCancellationWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	timeout time.Duration,
	batchSize int,
	logger *zap.Logger,
) *CancellationWorker {
	return &CancellationWorker{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		timeout:   timeout,
		batchSize: batchSize,
		logger:    logger,
	}
}

// Run запускает воркер. Блокирует горутину до отмены контекста.
func (w *CancellationWorker) Run(ctx context.Context) {
	w.logger.Info("воркер зависших отмен запущен",
		zap.Duration("interval", w.interval),
		zap.Duration("timeout", w.timeout),
		zap.Int("batchSize", w.batchSize),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер зависших отмен остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// processBatch обрабатывает один пакет зависших отмен.
func (w *CancellationWorker) processBatch(ctx context.Context) {
	threshold := time.Now().Add(-w.timeout)

	bookings, err := w.repo.GetStuckCancellations(ctx, threshold, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения зависших отмен", zap.Error(err))
		return
	}

	if len(bookings) == 0 {
		return
	}

	w.logger.Info("обработка зависших отмен", zap.Int("count", len(bookings)))

	for _, booking := range bookings {
		w.processBooking(ctx, &booking)
	}
}

// processBooking обрабатывает одну отмену, повторно отправляя событие в RabbitMQ.
func (w *CancellationWorker) processBooking(ctx context.Context, booking *models.Booking) {
	eventID := fmt.Sprintf("retry-cancel-booking-%d", booking.ID())

	cmd := messaging.CancelBookingJobCommand{
		EventId:   eventID,
		RequestId: messaging.BookingIDToRequestID(booking.ID()),
	}

	err := w.publisher.PublishCancelBookingJob(ctx, cmd)
	if err != nil {
		w.logger.Error("ошибка повторной отправки команды отмены",
			zap.Int64("bookingId", booking.ID()),
			zap.Error(err),
		)
		return
	}

	w.logger.Info("команда отмены повторно отправлена", zap.Int64("bookingId", booking.ID()))
}
