package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает ошибки/отклонения отмены бронирования.
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик ошибок отмены.
func NewCancelBookingErrorHandler(svc *service.BookingsService, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает событие BookingJobDenied.
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobDenied
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobDenied: %w", err)
	}
	bookingID, err := messaging.RequestIDToBookingID(event.RequestId)
	if err != nil {
		return fmt.Errorf("извлечение bookingId из RequestId: %w", err)
	}
	h.logger.Warn("получено событие BookingJobDenied, запуск отката",
		zap.Int64("bookingId", bookingID),
		zap.String("requestId", event.RequestId),
		zap.String("reason", event.Reason),
	)
	if err := h.service.HandleCancelError(ctx, event.RequestId); err != nil {
		return fmt.Errorf("ошибка отката для бронирования %d: %w", bookingID, err)
	}
	h.logger.Info("успешный откат", zap.Int64("bookingId", bookingID))
	return nil
}
