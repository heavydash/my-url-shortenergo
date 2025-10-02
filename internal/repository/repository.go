package repository

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

type URLRepository interface {
	SaveURL(m model.URLModel) (model.URLModel, error)
	GetURL(id string) (model.URLModel, error)
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
	if model.ID == "" {
		maxAttempts := 5
		for attempt := 0; attempt < maxAttempts; attempt++ {
			newID, err := idgen.IDGen()
			if err != nil {
				return model, fmt.Errorf("error generating uuid: %w", err)
			}
			if _, ok := m.urls[newID]; !ok {
				model.ID = newID
				m.urls[newID] = model
			}
		}
		return model, fmt.Errorf("failed to generate unique short ID after %d attempts", maxAttempts)
	} else {
		if _, ok := m.urls[model.ID]; !ok {
			return model, fmt.Errorf("model with id %s already exist", model.ID)
		}
		m.urls[model.ID] = model
	}
	return model, nil
}

func (m *MemoryRepository) GetURL(id string) (model.URLModel, error) {
	if _, ok := m.urls[id]; ok {
		return m.urls[id], nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}
