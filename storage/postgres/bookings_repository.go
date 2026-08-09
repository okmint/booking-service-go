package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// Create сохраняет новое бронирование.
func (r *BookingsRepository) Create(ctx context.Context, booking *models.Booking) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, queryInsertBooking,
		string(booking.Status()),
		booking.UserID(),
		booking.ResourceID(),
		booking.StartDate(),
		booking.EndDate(),
		booking.CreatedAt(),
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("создание бронирования: %w", err)
	}
	return id, nil
}

// GetByID возвращает бронирование по ID.
func (r *BookingsRepository) GetByID(ctx context.Context, id int64) (*models.Booking, error) {
	booking, err := r.scanBooking(r.pool.QueryRow(ctx, queryGetBookingByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrBookingNotFound
		}
		return nil, fmt.Errorf("получение бронирования id=%d: %w", id, err)
	}
	return booking, nil
}

// Update обновляет состояние бронирования.
func (r *BookingsRepository) Update(ctx context.Context, booking *models.Booking) error {
	var prevStatus *string
	if ps, ok := booking.PreviousStatus(); ok {
		s := string(ps)
		prevStatus = &s
	}

	var sentAt *time.Time
	if sa, ok := booking.CancelCommandSentAt(); ok {
		sentAt = &sa
	}

	tag, err := r.pool.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		prevStatus,
		sentAt,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrBookingNotFound
	}
	return nil
}

// GetByFilter возвращает бронирования с фильтрацией и пагинацией.
func (r *BookingsRepository) GetByFilter(ctx context.Context, filter models.BookingFilter) ([]models.Booking, int64, error) {
	offset := (filter.Page - 1) * filter.Size

	var userID, resourceID *int64
	var status *string
	if filter.UserID != nil {
		userID = filter.UserID
	}
	if filter.ResourceID != nil {
		resourceID = filter.ResourceID
	}
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}

	// Получение общего количества
	var totalCount int64
	err := r.pool.QueryRow(ctx, queryCountBookingsByFilter, userID, resourceID, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт бронирований: %w", err)
	}

	// Получение данных
	rows, err := r.pool.Query(ctx, queryGetBookingsByFilter, userID, resourceID, status, filter.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение бронирований по фильтру: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("итерация по строкам: %w", err)
	}

	return bookings, totalCount, nil
}

// GetAwaitingConfirmation возвращает бронирования, ожидающие подтверждения,
// с пессимистичной блокировкой FOR UPDATE SKIP LOCKED.
func (r *BookingsRepository) GetAwaitingConfirmation(ctx context.Context, limit int) ([]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetAwaitingConfirmation, limit)
	if err != nil {
		return nil, fmt.Errorf("получение бронирований для подтверждения: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	return bookings, rows.Err()
}

// scanBooking сканирует одну строку в доменный объект Booking.
func (r *BookingsRepository) scanBooking(row pgx.Row) (*models.Booking, error) {
	var (
		id              int64
		status          string
		userID          int64
		resourceID      int64
		startDate       time.Time
		endDate         time.Time
		createdAt       time.Time
		prevStatusStr   *string
		cancelCmdSentAt *time.Time
	)

	err := row.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &prevStatusStr, &cancelCmdSentAt)
	if err != nil {
		return nil, err
	}

	var prevStatus *models.BookingStatus
	if prevStatusStr != nil {
		ps := models.BookingStatus(*prevStatusStr)
		prevStatus = &ps
	}

	return models.RestoreBooking(id, models.BookingStatus(status), userID, resourceID, startDate, endDate, createdAt, prevStatus, cancelCmdSentAt), nil
}

// scanBookingFromRows сканирует строку из pgx.Rows.
func (r *BookingsRepository) scanBookingFromRows(rows pgx.Rows) (*models.Booking, error) {
	var (
		id              int64
		status          string
		userID          int64
		resourceID      int64
		startDate       time.Time
		endDate         time.Time
		createdAt       time.Time
		prevStatusStr   *string
		cancelCmdSentAt *time.Time
	)

	err := rows.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &prevStatusStr, &cancelCmdSentAt)
	if err != nil {
		return nil, err
	}

	var prevStatus *models.BookingStatus
	if prevStatusStr != nil {
		ps := models.BookingStatus(*prevStatusStr)
		prevStatus = &ps
	}

	return models.RestoreBooking(id, models.BookingStatus(status), userID, resourceID, startDate, endDate, createdAt, prevStatus, cancelCmdSentAt), nil
}

// GetStatistics возвращает агрегированную аналитику по бронированиям за период.
func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (models.BookingStatistics, error) {
	dateTo = dateTo.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	stats := models.BookingStatistics{
		Statuses:     make(map[string]int),
		TopResources: make([]models.ResourceStatistic, 0, 5),
	}

	rowsStatus, err := r.pool.Query(ctx, queryGetStatisticsByStatus, dateFrom, dateTo)
	if err != nil {
		return stats, fmt.Errorf("получение статистики по статусам: %w", err)
	}
	defer rowsStatus.Close()

	for rowsStatus.Next() {
		var status string
		var count int
		if err := rowsStatus.Scan(&status, &count); err != nil {
			return stats, fmt.Errorf("сканирование статуса: %w", err)
		}
		stats.Statuses[status] = count
		stats.TotalCount += count
	}
	if err := rowsStatus.Err(); err != nil {
		return stats, fmt.Errorf("итерация по статусам: %w", err)
	}

	rowsResources, err := r.pool.Query(ctx, queryGetStatisticsTopResources, dateFrom, dateTo)
	if err != nil {
		return stats, fmt.Errorf("получение статистики по ресурсам: %w", err)
	}
	defer rowsResources.Close()

	for rowsResources.Next() {
		var res models.ResourceStatistic
		if err := rowsResources.Scan(&res.ResourceID, &res.Count); err != nil {
			return stats, fmt.Errorf("сканирование ресурса: %w", err)
		}
		stats.TopResources = append(stats.TopResources, res)
	}
	if err := rowsResources.Err(); err != nil {
		return stats, fmt.Errorf("итерация по ресурсам: %w", err)
	}

	return stats, nil
}
