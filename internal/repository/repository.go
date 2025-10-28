package repository

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"log"
)

type URLRepository interface {
	SaveURL(m model.URLModel) (model.URLModel, error)
	GetURL(id string) (model.URLModel, error)
	Clear()
}

type MemoryRepository struct {
	urls map[string]model.URLModel
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		urls: make(map[string]model.URLModel),
	}
}

func (m *MemoryRepository) SaveURL(model model.URLModel) (model.URLModel, error) {
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
	if _, ok := m.urls[id]; ok {
		return m.urls[id], nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

func (m *MemoryRepository) Clear() {
	m.urls = make(map[string]model.URLModel)
	log.Printf("Cleared Urls, new size: %d", len(m.urls))
}
