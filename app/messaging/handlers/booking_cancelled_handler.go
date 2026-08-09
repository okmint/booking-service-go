package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// BookingCancelledHandler обрабатывает события успешной отмены из Catalog.
type BookingCancelledHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewBookingCancelledHandler создаёт новый обработчик.
func NewBookingCancelledHandler(svc *service.BookingsService, logger *zap.Logger) *BookingCancelledHandler {
	return &BookingCancelledHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает сообщение об успешной отмене.
func (h *BookingCancelledHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobCancelled
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobCancelled: %w", err)
	}

	h.logger.Info("получено подтверждение отмены от Catalog, завершение отмены",
		zap.String("requestId", event.RequestId),
		zap.String("eventId", event.EventId),
	)
	// Catalog подтвердил отмену — переводим бронь в cancelled.
	if err := h.service.CompleteCancellation(ctx, event.RequestId); err != nil {
		return fmt.Errorf("ошибка завершения отмены для requestId %s: %w", event.RequestId, err)
	}

	return nil
}
