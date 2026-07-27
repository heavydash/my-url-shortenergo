// Package service предоставляет бизнес-логику для работы с URL.
// Выступает в качестве промежуточного слоя между HTTP-обработчиками и репозиторием.
// Позволяет добавлять бизнес-правила, кэширование, валидацию и другие
// сквозные задачи без изменения интерфейса репозитория.
package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

// URLServiceImpl реализует интерфейс URLService.
// Является тонкой прослойкой над репозиторием без дополнительной логики.
type URLServiceImpl struct {
	repo repository.URLRepository
}

// NewURLService создаёт новый экземпляр URLService.
//
// Параметры:
//   - repo: репозиторий для хранения URL
//
// Возвращает:
//   - URLService: готовый к использованию сервис
func NewURLService(repo repository.URLRepository) URLService {
	return &URLServiceImpl{
		repo: repo,
	}
}

// SaveURL сохраняет URL в хранилище.
func (s *URLServiceImpl) SaveURL(ctx context.Context, m model.URLModel) (*model.URLModel, error) {
	return s.repo.SaveURL(ctx, m)
}

// GetURL возвращает URL по его короткому идентификатору.
func (s *URLServiceImpl) GetURL(ctx context.Context, id string) (*model.URLModel, error) {
	return s.repo.GetURL(ctx, id)
}

// SaveBatch сохраняет несколько URL одной операцией.
func (s *URLServiceImpl) SaveBatch(ctx context.Context, batch []model.URLModel) error {
	return s.repo.SaveBatch(ctx, batch)
}

// GetURLsByUser возвращает все URL принадлежащие указанному пользователю.
func (s *URLServiceImpl) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {
	return s.repo.GetURLsByUser(ctx, userID)
}

// MarkAsDeleted помечает указанные URL как удаленные.
func (s *URLServiceImpl) MarkAsDeleted(ctx context.Context, userID uuid.UUID, shortURLs []string) error {
	return s.repo.MarkAsDeleted(ctx, userID, shortURLs)
}

// Stats возвращает общую статистику сервиса.
func (s *URLServiceImpl) Stats() (urls int, users int) {
	return s.repo.Stats()
}

// Ping проверяет доступность хранилища.
func (s *URLServiceImpl) Ping(ctx context.Context) error {
	return s.repo.Ping(ctx)
}

// Clear очищает всё хранилище.
func (s *URLServiceImpl) Clear() error {
	return s.repo.Clear()
}
