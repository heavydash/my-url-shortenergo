package repository

import (
	"fmt"
	"github.com/google/uuid"
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
		var uuidVal uuid.UUID
		var err error
		for {
			uuidVal, err = uuid.NewRandom()
			if err != nil {
				return model, fmt.Errorf("error generating uuid: %w", err)
			}
			newuuid := uuidVal.String()
			if _, ok := m.urls[newuuid]; !ok {
				model.ID = newuuid
				m.urls[newuuid] = model
				break
			}
		}
	} else {
		if _, ok := m.urls[model.ID]; !ok {
			return model, fmt.Errorf("model with id %s does not exist", model.ID)
		}
		if len(model.ID) != 36 {
			return model, fmt.Errorf("invalid id format: %s", model.ID)
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
