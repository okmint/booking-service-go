package models

import "errors"

var (
	ErrInvalidStatusTransition = errors.New("недопустимый переход статуса")
	ErrCannotCancelPastBooking = errors.New("нельзя отменить бронирование с прошедшей датой начала")
	ErrInvalidUserID           = errors.New("некорректный ID пользователя")
	ErrInvalidResourceID       = errors.New("некорректный ID ресурса")
	ErrInvalidDateRange        = errors.New("некорректный диапазон дат")
	ErrEndDateBeforeStartDate  = errors.New("дата окончания раньше даты начала")
	ErrBookingNotFound         = errors.New("бронирование не найдено")
	ErrEventAlreadyProcessed   = errors.New("событие уже обработано")
)
