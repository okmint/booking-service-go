package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
//
// Этот сервис -- оркестратор: он координирует домен и репозиторий,
// но НЕ содержит бизнес-правила (они в models.Booking).
type BookingsService struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	logger    *zap.Logger
}

// NewBookingsService создаёт новый BookingsService.
func NewBookingsService(repo models.BookingRepository, publisher *messaging.Publisher, logger *zap.Logger) *BookingsService {
	return &BookingsService{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
	}
}

// buildDomainEventPayload формирует и сериализует событие изменения статуса для сохранения в Outbox.
func (s *BookingsService) buildDomainEventPayload(id int64, oldStatus, newStatus models.BookingStatus, reason string) ([]byte, error) {
	if oldStatus == newStatus {
		return nil, nil
	}

	event := messaging.BookingStatusChangedEvent{
		EventId:   messaging.NewMessageID(),
		BookingId: id,
		OldStatus: string(oldStatus),
		NewStatus: string(newStatus),
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	return json.Marshal(event)
}

// Create создаёт новое бронирование.
func (s *BookingsService) Create(ctx context.Context, req dto.CreateBookingRequest) (int64, error) {
	startDate, err := time.Parse(dto.DateFormat, req.StartDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат startDate: %w", err)
	}

	endDate, err := time.Parse(dto.DateFormat, req.EndDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат endDate: %w", err)
	}

	booking, err := models.NewBooking(req.UserID, req.ResourceID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	initiator := fmt.Sprint(req.UserID)
	id, err := s.repo.Create(ctx, booking, initiator, "создание бронирования")
	if err != nil {
		return 0, fmt.Errorf("сохранение бронирования: %w", err)
	}

	s.logger.Info("бронирование создано",
		zap.Int64("id", id),
		zap.Int64("userId", req.UserID),
		zap.Int64("resourceId", req.ResourceID),
	)

	if err := s.publisher.PublishCreateBookingJob(ctx, messaging.CreateBookingJobCommand{
		EventId:    messaging.NewMessageID(),
		RequestId:  messaging.BookingIDToRequestID(id),
		ResourceId: req.ResourceID,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	}); err != nil {
		s.logger.Error("ошибка публикации CreateBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return id, nil
}

// Cancel инициирует процесс отмены бронирования по ID.
func (s *BookingsService) Cancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldStatus := booking.Status()
	reason := "инициация отмены"

	if err := booking.InitiateCancellation(time.Now()); err != nil {
		return err
	}

	payload, err := s.buildDomainEventPayload(id, oldStatus, booking.Status(), reason)
	if err != nil {
		return fmt.Errorf("сериализация outbox payload: %w", err)
	}

	initiator := fmt.Sprint(booking.UserID())
	if err := s.repo.Update(ctx, booking, initiator, reason, payload); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("начата отмена бронирования", zap.Int64("id", id))

	if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(id),
	}); err != nil {
		s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return nil
}

// CancelWithEvent инициирует отмену бронирования с защитой идемпотентности.
func (s *BookingsService) CancelWithEvent(ctx context.Context, id int64, eventID string) error {
	processed, err := s.repo.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		s.logger.Warn("событие уже обработано, пропускаем отмену", zap.String("event_id", eventID))
		return nil
	}

	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldStatus := booking.Status()
	reason := "отказ каталога"

	if err := booking.InitiateCancellation(time.Now()); err != nil {
		return err
	}

	payload, err := s.buildDomainEventPayload(id, oldStatus, booking.Status(), reason)
	if err != nil {
		return fmt.Errorf("сериализация outbox payload: %w", err)
	}

	initiator := "CatalogSystem"
	err = s.repo.UpdateWithEvent(ctx, booking, initiator, reason, eventID, payload)
	if err != nil {
		if errors.Is(err, models.ErrEventAlreadyProcessed) {
			s.logger.Warn("дубликат события проигнорирован при обновлении", zap.String("event_id", eventID))
			return nil
		}
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("начата отмена бронирования (отказ каталога)", zap.Int64("id", id))

	if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(id),
	}); err != nil {
		s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return nil
}

// CompleteCancellation подтверждает успешную отмену с защитой идемпотентности.
func (s *BookingsService) CompleteCancellation(ctx context.Context, requestID, eventID string) error {
	processed, err := s.repo.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		s.logger.Warn("событие уже обработано, пропускаем завершение отмены", zap.String("event_id", eventID))
		return nil
	}

	id, err := messaging.RequestIDToBookingID(requestID)
	if err != nil {
		return fmt.Errorf("некорректный requestID: %w", err)
	}

	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrBookingNotFound) {
			s.logger.Warn("бронирование не найдено, игнорируем завершение отмены", zap.Int64("id", id))
			return nil
		}
		return fmt.Errorf("получение бронирования: %w", err)
	}

	oldStatus := booking.Status()
	reason := "отмена успешно завершена"

	if err := booking.CompleteCancellation(); err != nil {
		if errors.Is(err, models.ErrInvalidStatusTransition) {
			s.logger.Warn("завершение отмены проигнорировано", zap.Int64("id", id))
			return nil
		}
		return fmt.Errorf("завершение отмены бронирования %d: %w", id, err)
	}

	payload, err := s.buildDomainEventPayload(id, oldStatus, booking.Status(), reason)
	if err != nil {
		return fmt.Errorf("сериализация outbox payload: %w", err)
	}

	err = s.repo.UpdateWithEvent(ctx, booking, "System", reason, eventID, payload)
	if err != nil {
		if errors.Is(err, models.ErrEventAlreadyProcessed) {
			s.logger.Warn("дубликат события проигнорирован при обновлении", zap.String("event_id", eventID))
			return nil
		}
		return fmt.Errorf("сохранение завершённой отмены: %w", err)
	}

	s.logger.Info("бронирование отменено", zap.Int64("id", id))
	return nil
}

// HandleCancelError выполняет компенсирующую транзакцию с защитой идемпотентности.
func (s *BookingsService) HandleCancelError(ctx context.Context, requestID, eventID string) error {
	processed, err := s.repo.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		s.logger.Warn("событие уже обработано, пропускаем откат", zap.String("event_id", eventID))
		return nil
	}

	id, err := messaging.RequestIDToBookingID(requestID)
	if err != nil {
		return fmt.Errorf("невалидный requestID: %w", err)
	}

	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrBookingNotFound) {
			s.logger.Warn("бронирование не найдено, игнорируем откат отмены", zap.Int64("id", id))
			return nil
		}
		return fmt.Errorf("получение бронирования для отката: %w", err)
	}

	oldStatus := booking.Status()
	reason := "откат отмены"

	if err := booking.RollbackCancellation(); err != nil {
		if errors.Is(err, models.ErrInvalidStatusTransition) {
			s.logger.Warn("откат отменён, неверный статус", zap.Int64("id", id))
			return nil
		}
		return fmt.Errorf("откат отмены бронирования %d: %w", id, err)
	}

	payload, err := s.buildDomainEventPayload(id, oldStatus, booking.Status(), reason)
	if err != nil {
		return fmt.Errorf("сериализация outbox payload: %w", err)
	}

	err = s.repo.UpdateWithEvent(ctx, booking, "System", reason, eventID, payload)
	if err != nil {
		if errors.Is(err, models.ErrEventAlreadyProcessed) {
			s.logger.Warn("дубликат события проигнорирован при обновлении", zap.String("event_id", eventID))
			return nil
		}
		return fmt.Errorf("сохранение отката: %w", err)
	}

	s.logger.Info("успешный откат", zap.Int64("id", id))
	return nil
}

// Confirm подтверждает бронирование по ID с защитой идемпотентности.
func (s *BookingsService) Confirm(ctx context.Context, id int64, eventID string) (bool, error) {
	processed, err := s.repo.IsProcessed(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		s.logger.Warn("событие уже обработано, пропускаем подтверждение", zap.String("event_id", eventID))
		return false, nil
	}

	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}

	isRaceCondition := booking.Status() == models.BookingStatusCancellationPending
	oldStatus := booking.Status()
	reason := "подтверждено каталогом"

	if err := booking.Confirm(); err != nil {
		return false, err
	}

	payload, err := s.buildDomainEventPayload(id, oldStatus, booking.Status(), reason)
	if err != nil {
		return false, fmt.Errorf("сериализация outbox payload: %w", err)
	}

	err = s.repo.UpdateWithEvent(ctx, booking, "System", reason, eventID, payload)
	if err != nil {
		if errors.Is(err, models.ErrEventAlreadyProcessed) {
			s.logger.Warn("дубликат события проигнорирован при обновлении", zap.String("event_id", eventID))
			return false, nil
		}
		return false, fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("состояние бронирования обновлено", zap.Int64("id", id))
	return isRaceCondition, nil
}
