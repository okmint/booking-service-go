package worker

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// OutboxWorker -- фоновый воркер для реализации Transactional Outbox.
//
// Логика работы:
//  1. Найти сообщения в статусе 'pending' с блокировкой FOR UPDATE SKIP LOCKED.
//  2. Десериализовать payload и попытаться отправить в RabbitMQ.
//  3. При успехе -- отметить как 'processed'.
//  4. При ошибке -- увеличить retry_count. Если превышен лимит, отметить как 'failed'.
type OutboxWorker struct {
	repo       models.BookingRepository
	publisher  *messaging.Publisher
	interval   time.Duration
	batchSize  int
	maxRetries int
	logger     *zap.Logger
}

// NewOutboxWorker создаёт новый воркер для обработки исходящих сообщений.
func NewOutboxWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	batchSize int,
	maxRetries int,
	logger *zap.Logger,
) *OutboxWorker {
	return &OutboxWorker{
		repo:       repo,
		publisher:  publisher,
		interval:   interval,
		batchSize:  batchSize,
		maxRetries: maxRetries,
		logger:     logger,
	}
}

// Run запускает воркер и блокирует горутину до отмены контекста.
func (w *OutboxWorker) Run(ctx context.Context) {
	w.logger.Info("воркер outbox запущен",
		zap.Duration("interval", w.interval),
		zap.Int("batchSize", w.batchSize),
		zap.Int("maxRetries", w.maxRetries),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер outbox остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

// processBatch обрабатывает один пакет сообщений из outbox.
func (w *OutboxWorker) processBatch(ctx context.Context) {
	messages, err := w.repo.GetPendingOutboxMessages(ctx, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения сообщений outbox", zap.Error(err))
		return
	}

	if len(messages) == 0 {
		return
	}

	w.logger.Debug("обработка сообщений outbox", zap.Int("count", len(messages)))

	for _, msg := range messages {
		w.processMessage(ctx, msg)
	}
}

// processMessage обрабатывает одно сообщение.
func (w *OutboxWorker) processMessage(ctx context.Context, msg models.OutboxMessage) {
	logger := w.logger.With(
		zap.Int64("messageId", msg.ID),
		zap.String("eventType", msg.EventType),
	)

	var event messaging.BookingStatusChangedEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		logger.Error("ошибка десериализации payload, помечаем как failed", zap.Error(err))
		_ = w.repo.UpdateOutboxMessage(ctx, msg.ID, "failed", msg.RetryCount, nil)
		return
	}

	err := w.publisher.PublishBookingStatusChangedEvent(ctx, event)
	if err == nil {
		now := time.Now()
		if dbErr := w.repo.UpdateOutboxMessage(ctx, msg.ID, "processed", msg.RetryCount, &now); dbErr != nil {
			logger.Error("сообщение опубликовано, но не удалось обновить статус в БД", zap.Error(dbErr))
		} else {
			logger.Debug("сообщение успешно опубликовано и отмечено как обработанное")
		}
		return
	}

	newRetryCount := msg.RetryCount + 1
	newStatus := "pending"
	if newRetryCount >= w.maxRetries {
		newStatus = "failed"
		logger.Error("превышен лимит попыток публикации", zap.Error(err), zap.Int("retries", newRetryCount))
	} else {
		logger.Warn("ошибка публикации сообщения, будет повторная попытка", zap.Error(err), zap.Int("retries", newRetryCount))
	}

	if dbErr := w.repo.UpdateOutboxMessage(ctx, msg.ID, newStatus, newRetryCount, nil); dbErr != nil {
		logger.Error("не удалось обновить статус ошибки в БД", zap.Error(dbErr))
	}
}
