package audit

import (
	"time"
)

// Event структура - описывает одно событие аудита
type Event struct {
	Timestamp int64  `json:"ts"`
	Action    string `json:"action"`
	UserID    string `json:"user_id,omitempty"`
	URL       string `json:"url"` // оригинальный длинный
}

// Конструктор для события создания короткой ссылки
func NewShortenEvent(userID, originalURL string) *Event {
	return &Event{
		Timestamp: time.Now().Unix(),
		Action:    "shorten",
		UserID:    userID,
		URL:       originalURL,
	}
}

// Конструктор для события перехода по короткой ссылке
func NewFollowEvent(userID, originalURL string) *Event {
	return &Event{
		Timestamp: time.Now().Unix(),
		Action:    "follow",
		UserID:    userID,
		URL:       originalURL,
	}
}
