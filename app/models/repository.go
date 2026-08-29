package models

import (
	"context"
	"time"
)

// BookingHistoryEntry -- запись лога бронирования.
type BookingHistoryEntry struct {
	ID             int64          `json:"id"`
	BookingID      int64          `json:"bookingId"`
	PreviousStatus *BookingStatus `json:"previousStatus"`
	NewStatus      BookingStatus  `json:"newStatus"`
	Initiator      string         `json:"initiator"`
	Reason         string         `json:"reason"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// BookingRepository -- интерфейс репозитория бронирований.
type BookingRepository interface {
	// Create сохраняет новое бронирование и пишет историю в одной транзакции.
	Create(ctx context.Context, booking *Booking, initiator, reason string) (int64, error)

	// GetByID возвращает бронирование по ID.
	GetByID(ctx context.Context, id int64) (*Booking, error)

	// Update обновляет бронирование и пишет историю в одной транзакции.
	Update(ctx context.Context, booking *Booking, initiator, reason string) error

	// UpdateWithEvent обновляет бронирование, пишет историю и фиксирует eventID в одной транзакции.
	// Возвращает ErrEventAlreadyProcessed при попытке обработать дубликат.
	UpdateWithEvent(ctx context.Context, booking *Booking, initiator, reason, eventID string) error

	// GetHistory возвращает историю статусов конкретного бронирования с пагинацией.
	GetHistory(ctx context.Context, bookingID int64, page, size int) ([]BookingHistoryEntry, int64, error)

	// GetByFilter возвращает список бронирований с пагинацией.
	GetByFilter(ctx context.Context, filter BookingFilter) ([]Booking, int64, error)

	// GetAwaitingConfirmation возвращает бронирования в статусе AwaitsConfirmation
	// с пессимистичной блокировкой (SELECT ... FOR UPDATE SKIP LOCKED).
	GetAwaitingConfirmation(ctx context.Context, limit int) ([]Booking, error)

	// GetStatistics возвращает агрегированную статистику бронирований.
	GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (BookingStatistics, error)

	// GetStuckCancellations возвращает бронирования, зависшие в статусе отмены дольше заданного таймаута.
	GetStuckCancellations(ctx context.Context, threshold time.Time, limit int) ([]Booking, error)
}

// BookingFilter содержит параметры фильтрации и пагинации.
type BookingFilter struct {
	UserID     *int64
	ResourceID *int64
	Status     *BookingStatus
	Page       int
	Size       int
}

// NewDefaultFilter создаёт фильтр с пагинацией по умолчанию.
func NewDefaultFilter() BookingFilter {
	return BookingFilter{
		Page: 1,
		Size: 25,
	}
}
