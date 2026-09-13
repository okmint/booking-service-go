package models

import "time"

// OutboxMessage представляет сообщение, готовое к асинхронной отправке в брокер.
type OutboxMessage struct {
	ID         int64
	EventType  string
	Payload    []byte
	Status     string
	RetryCount int
	CreatedAt  time.Time
}
