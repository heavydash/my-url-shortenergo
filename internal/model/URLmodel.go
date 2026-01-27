package model

import "github.com/google/uuid"

type URLModel struct {
	ID          uuid.UUID `db:"id"`
	UUID        string    `json:"uuid,omitempty"`
	ShortURL    string    `json:"short_url"`
	OriginalURL string    `json:"original_url"`
	UserID      uuid.UUID `json:"-" db:"omitempty"`
	IsDeleted   bool      `json:"-" db:"is_deleted"`
}
