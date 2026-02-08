package repository

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

type MemoryRepository struct {
	mu   sync.Mutex
	urls map[string]model.URLModel
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		urls: make(map[string]model.URLModel),
	}
}

func (m *MemoryRepository) SaveURL(model model.URLModel) (model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newModel := model
	if newModel.UUID != "" {
		if _, ok := m.urls[newModel.UUID]; ok {
			return model, fmt.Errorf("url with id %s already exists", newModel.UUID)
		}
		m.urls[newModel.UUID] = newModel
		return newModel, nil
	}
	maxAttempts := 5
	log.Printf("Attempting to save URL, current Urls size: %d", len(m.urls))
	for attempt := 0; attempt < maxAttempts; attempt++ {
		newID, err := idgen.IDGen()
		if err != nil {
			return model, fmt.Errorf("error generating uuid: %w", err)
		}
		currentModel := model
		log.Printf("Attempt %d, checking ID: %s, Urls size: %d", attempt, newID, len(m.urls))
		if _, ok := m.urls[newID]; !ok {
			currentModel.UUID = newID
			m.urls[newID] = currentModel
			return currentModel, nil
		}
	}
	return model, fmt.Errorf("failed to generate unique short ID after %d attempts", maxAttempts)
}

func (m *MemoryRepository) GetURL(id string) (model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.urls[id]; ok {
		return m.urls[id], nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

func (m *MemoryRepository) SaveBatch(сtx context.Context, batch []model.URLModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range batch {
		m.urls[item.UUID] = item
	}
	return nil
}

func (m *MemoryRepository) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls = make(map[string]model.URLModel)
	log.Printf("Cleared Urls, new size: %d", len(m.urls))
	return nil
}

func (m *MemoryRepository) Ping(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}

func (m *MemoryRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if userID == uuid.Nil {
		return []model.URLModel{}, nil
	}

	var result []model.URLModel
	baseURL := "http://localhost:8080"

	for _, m := range m.urls {
		if m.UserID == userID && !m.IsDeleted {
			result = append(result, model.URLModel{
				ShortURL:    fmt.Sprintf("%s/%s", baseURL, m.ShortURL),
				OriginalURL: m.OriginalURL,
			})
		}
	}
	return result, nil
}

func (m *MemoryRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, shortURL := range shortURLs {
		if model, ok := m.urls[shortURL]; ok && model.UserID == userID {
			model.IsDeleted = true
			m.urls[shortURL] = model
		}
	}
	return nil
}
