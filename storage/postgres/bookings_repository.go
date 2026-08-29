package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

var ErrEventAlreadyProcessed = errors.New("событие уже обработано")

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// Create сохраняет новое бронирование и пишет историю в одной транзакции.
func (r *BookingsRepository) Create(ctx context.Context, booking *models.Booking, initiator, reason string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var id int64
	err = tx.QueryRow(ctx, queryInsertBooking,
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

	_, err = tx.Exec(ctx, queryInsertHistory,
		id,
		nil,
		string(booking.Status()),
		initiator,
		reason,
	)
	if err != nil {
		return 0, fmt.Errorf("сохранение истории: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("коммит транзакции: %w", err)
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

// Update обновляет состояние бронирования и пишет лог.
func (r *BookingsRepository) Update(ctx context.Context, booking *models.Booking, initiator, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var oldStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM bookings WHERE id = $1 FOR UPDATE", booking.ID()).Scan(&oldStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrBookingNotFound
		}
		return fmt.Errorf("получение старого статуса: %w", err)
	}

	var prevStatus *string
	if ps, ok := booking.PreviousStatus(); ok {
		s := string(ps)
		prevStatus = &s
	}
	var sentAt *time.Time
	if sa, ok := booking.CancelCommandSentAt(); ok {
		sentAt = &sa
	}

	_, err = tx.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		prevStatus,
		sentAt,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}

	if oldStatus != string(booking.Status()) {
		_, err = tx.Exec(ctx, queryInsertHistory,
			booking.ID(),
			oldStatus,
			string(booking.Status()),
			initiator,
			reason,
		)
		if err != nil {
			return fmt.Errorf("сохранение истории: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// UpdateWithEvent обновляет состояние бронирования, пишет лог и фиксирует eventID в одной транзакции.
// Если eventID уже существует, возвращает ErrEventAlreadyProcessed.
func (r *BookingsRepository) UpdateWithEvent(ctx context.Context, booking *models.Booking, initiator, reason, eventID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	_, err = tx.Exec(ctx, queryInsertProcessedEvent, eventID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEventAlreadyProcessed
		}
		return fmt.Errorf("фиксация обработанного события: %w", err)
	}

	var oldStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM bookings WHERE id = $1 FOR UPDATE", booking.ID()).Scan(&oldStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrBookingNotFound
		}
		return fmt.Errorf("получение старого статуса: %w", err)
	}

	var prevStatus *string
	if ps, ok := booking.PreviousStatus(); ok {
		s := string(ps)
		prevStatus = &s
	}
	var sentAt *time.Time
	if sa, ok := booking.CancelCommandSentAt(); ok {
		sentAt = &sa
	}

	_, err = tx.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		prevStatus,
		sentAt,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}

	if oldStatus != string(booking.Status()) {
		_, err = tx.Exec(ctx, queryInsertHistory,
			booking.ID(),
			oldStatus,
			string(booking.Status()),
			initiator,
			reason,
		)
		if err != nil {
			return fmt.Errorf("сохранение истории: %w", err)
		}
	}

	return tx.Commit(ctx)
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

// GetStuckCancellations возвращает бронирования, зависшие в статусе отмены дольше заданного таймаута.
func (r *BookingsRepository) GetStuckCancellations(ctx context.Context, threshold time.Time, limit int) ([]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetStuckCancellations, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("получение зависших отмен: %w", err)
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

// GetStatistics возвращает агрегированную аналитику по бронированиям за период.
func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (models.BookingStatistics, error) {
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

// GetHistory возвращает историю статусов конкретного бронирования.
func (r *BookingsRepository) GetHistory(ctx context.Context, bookingID int64, page, size int) ([]models.BookingHistoryEntry, int64, error) {
	offset := (page - 1) * size

	var totalCount int64
	if err := r.pool.QueryRow(ctx, queryCountHistory, bookingID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("подсчет записей истории: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryGetHistory, bookingID, size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение истории: %w", err)
	}
	defer rows.Close()

	var history []models.BookingHistoryEntry
	for rows.Next() {
		var entry models.BookingHistoryEntry
		var prevStatusStr *string
		var newStatusStr string

		err := rows.Scan(
			&entry.ID,
			&entry.BookingID,
			&prevStatusStr,
			&newStatusStr,
			&entry.Initiator,
			&entry.Reason,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование истории: %w", err)
		}

		if prevStatusStr != nil {
			ps := models.BookingStatus(*prevStatusStr)
			entry.PreviousStatus = &ps
		}
		entry.NewStatus = models.BookingStatus(newStatusStr)

		history = append(history, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("итерация по строкам истории: %w", err)
	}

	return history, totalCount, nil
}
