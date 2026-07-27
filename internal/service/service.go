// Package service предоставляет бизнес-логику для работы с URL.
// Выступает в качестве промежуточного слоя между HTTP-обработчиками и репозиторием.
package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

// URLService определяет интерфейс для работы с URL на бизнес-уровне.
// Повторяет методы репозитория, но в будущем может расширяться
// дополнительной логикой (например, валидация URL перед сохранением).
type URLService interface {
	// SaveURL сохраняет URL в хранилище.
	SaveURL(ctx context.Context, m model.URLModel) (*model.URLModel, error)

	// GetURL возвращает URL по его короткому идентификатору.
	GetURL(ctx context.Context, id string) (*model.URLModel, error)

	// SaveBatch сохраняет несколько URL одной операцией.
	SaveBatch(ctx context.Context, batch []model.URLModel) error

	// GetURLsByUser возвращает все URL принадлежащие указанному пользователю.
	GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error)

	// MarkAsDeleted помечает указанные URL как удаленные.
	MarkAsDeleted(ctx context.Context, userID uuid.UUID, shortURLs []string) error

	// Clear очищает всё хранилище.
	Clear() error

	// Stats возвращает общую статистику сервиса (количество URL и уникальных пользователей).
	Stats() (urls int, users int)

	// Ping проверяет доступность хранилища.
	Ping(ctx context.Context) error
}
