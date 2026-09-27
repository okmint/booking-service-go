package notifications

import "time"

// NotificationRequest описывает тело запроса к сервису уведомлений.
type NotificationRequest struct {
	BookingID int64     `json:"booking_id"`
	UserID    int64     `json:"user_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	ChangedAt time.Time `json:"changed_at"`
}
