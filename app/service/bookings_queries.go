package service

import (
	"context"
	"fmt"
	"time"

	"booking-service/app/api/dto"
	"booking-service/app/models"

	"github.com/patrickmn/go-cache"
	"go.uber.org/zap"
)

// BookingsQueries обрабатывает запросы (чтение данных) для бронирований.
type BookingsQueries struct {
	repo       models.BookingRepository
	statsCache *cache.Cache
	logger     *zap.Logger
}

// NewBookingsQueries создаёт новый BookingsQueries.
func NewBookingsQueries(repo models.BookingRepository, statsCache *cache.Cache, logger *zap.Logger) *BookingsQueries {
	return &BookingsQueries{
		repo:       repo,
		statsCache: statsCache,
		logger:     logger,
	}
}

// GetByID возвращает бронирование по ID.
func (q *BookingsQueries) GetByID(ctx context.Context, id int64) (dto.BookingResponse, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return dto.BookingResponse{}, err
	}

	return mapBookingToResponse(booking), nil
}

// GetStatus возвращает статус бронирования по ID.
func (q *BookingsQueries) GetStatus(ctx context.Context, id int64) (models.BookingStatus, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return booking.Status(), nil
}

// GetByFilter возвращает список бронирований с пагинацией.
func (q *BookingsQueries) GetByFilter(ctx context.Context, req dto.GetBookingsByFilterRequest) (dto.PagedResponse[dto.BookingResponse], error) {
	filter := models.NewDefaultFilter()

	if req.Page > 0 {
		filter.Page = req.Page
	}
	if req.Size > 0 {
		filter.Size = req.Size
	}
	if req.UserID != nil {
		filter.UserID = req.UserID
	}
	if req.ResourceID != nil {
		filter.ResourceID = req.ResourceID
	}
	if req.Status != nil {
		status := models.BookingStatus(*req.Status)
		if !status.IsValid() {
			return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("некорректный статус: %s", *req.Status)
		}
		filter.Status = &status
	}

	bookings, totalCount, err := q.repo.GetByFilter(ctx, filter)
	if err != nil {
		return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("получение бронирований: %w", err)
	}

	items := make([]dto.BookingResponse, 0, len(bookings))
	for i := range bookings {
		items = append(items, mapBookingToResponse(&bookings[i]))
	}

	return dto.PagedResponse[dto.BookingResponse]{
		Items:      items,
		TotalCount: totalCount,
		Page:       filter.Page,
		Size:       filter.Size,
	}, nil
}

// GetHistory возвращает историю статусов бронирования с пагинацией.
func (q *BookingsQueries) GetHistory(ctx context.Context, id int64, page, size int) (dto.PagedResponse[dto.BookingHistoryResponse], error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 25
	}

	history, totalCount, err := q.repo.GetHistory(ctx, id, page, size)
	if err != nil {
		return dto.PagedResponse[dto.BookingHistoryResponse]{}, fmt.Errorf("получение истории: %w", err)
	}

	items := make([]dto.BookingHistoryResponse, 0, len(history))
	for i := range history {
		items = append(items, mapHistoryToResponse(&history[i]))
	}

	return dto.PagedResponse[dto.BookingHistoryResponse]{
		Items:      items,
		TotalCount: totalCount,
		Page:       page,
		Size:       size,
	}, nil
}

// mapBookingToResponse конвертирует доменный объект в DTO ответа.
func mapBookingToResponse(b *models.Booking) dto.BookingResponse {
	return dto.BookingResponse{
		ID:         b.ID(),
		Status:     string(b.Status()),
		UserID:     b.UserID(),
		ResourceID: b.ResourceID(),
		StartDate:  b.StartDate().Format(dto.DateFormat),
		EndDate:    b.EndDate().Format(dto.DateFormat),
		CreatedAt:  b.CreatedAt().Format(time.RFC3339),
	}
}

// mapHistoryToResponse конвертирует запись истории в DTO ответа.
func mapHistoryToResponse(h *models.BookingHistoryEntry) dto.BookingHistoryResponse {
	var prevStatus *string
	if h.PreviousStatus != nil {
		s := string(*h.PreviousStatus)
		prevStatus = &s
	}

	return dto.BookingHistoryResponse{
		ID:             h.ID,
		BookingID:      h.BookingID,
		PreviousStatus: prevStatus,
		NewStatus:      string(h.NewStatus),
		Initiator:      h.Initiator,
		Reason:         h.Reason,
		CreatedAt:      h.CreatedAt.Format(time.RFC3339),
	}
}

// GetStatistics возвращает агрегированную аналитику за указанный период.
func (q *BookingsQueries) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (dto.BookingStatisticsResponse, error) {
	cacheKey := fmt.Sprintf("stats_%s_%s", dateFrom.Format(time.RFC3339), dateTo.Format(time.RFC3339))

	if cachedData, found := q.statsCache.Get(cacheKey); found {
		q.logger.Debug("статистика получена из кэша", zap.String("key", cacheKey))
		return cachedData.(dto.BookingStatisticsResponse), nil
	}

	stats, err := q.repo.GetStatistics(ctx, dateFrom, dateTo)
	if err != nil {
		return dto.BookingStatisticsResponse{}, fmt.Errorf("получение статистики: %w", err)
	}

	statuses := map[string]int{
		string(models.BookingStatusAwaitsConfirmation):  0,
		string(models.BookingStatusConfirmed):           0,
		string(models.BookingStatusCancellationPending): 0,
		string(models.BookingStatusCancelled):           0,
	}

	for status, count := range stats.Statuses {
		statuses[status] = count
	}

	topResourcesDTO := make([]dto.ResourceStatistic, 0, len(stats.TopResources))
	for _, res := range stats.TopResources {
		topResourcesDTO = append(topResourcesDTO, dto.ResourceStatistic{
			ResourceID: res.ResourceID,
			Count:      res.Count,
		})
	}

	response := dto.BookingStatisticsResponse{
		TotalCount:   stats.TotalCount,
		Statuses:     statuses,
		TopResources: topResourcesDTO,
	}

	q.statsCache.Set(cacheKey, response, cache.DefaultExpiration)
	q.logger.Debug("статистика сохранена в кэш", zap.String("key", cacheKey))

	return response, nil
}
