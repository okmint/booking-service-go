package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает сообщения из DLQ.
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик.
func NewCancelBookingErrorHandler(svc *service.BookingsService, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает сообщение из DLQ.
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.CancelBookingJobCommand
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация CancelBookingJobCommand: %w", err)
	}

	h.logger.Warn("получено сообщение из DLQ (ошибка отмены), передаем в сервис для отката",
		zap.String("requestId", event.RequestId),
		zap.String("eventId", event.EventId),
	)

	if err := h.service.HandleCancelError(ctx, event.RequestId, event.EventId); err != nil {
		return fmt.Errorf("ошибка обработки отката для requestId %s: %w", event.RequestId, err)
	}
	return nil
}
