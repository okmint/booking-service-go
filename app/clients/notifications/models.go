package notifications

// NotificationRequest описывает тело запроса к сервису уведомлений.
type NotificationRequest struct {
	BookingID int64  `json:"booking_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}
