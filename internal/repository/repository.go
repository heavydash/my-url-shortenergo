package repository

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

type URLRepository interface {
	SaveURL(m model.URLModel) (model.URLModel, error)
	GetURL(id string) (model.URLModel, error)
}

type MemoryRepository struct {
	urls    []model.URLModel
	counter int
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		urls:    make([]model.URLModel, 0),
		counter: 0,
	}
}

func (m *MemoryRepository) SaveURL(model model.URLModel) (model.URLModel, error) {
	if model.ID == "" {
		m.counter++
		model.ID = fmt.Sprintf("%08d", m.counter-1)
	} else {
		if len(model.ID) != 8 || model.ID[0] != '0' {
			return model, fmt.Errorf("invalid ID format")
		}
		idInt, _ := fmt.Sscanf(model.ID, "%08d", new(int))
		if idInt >= m.counter {
			m.counter = idInt + 1
		}
	}
	m.urls = append(m.urls, model)
	return model, nil
}
func (m *MemoryRepository) GetURL(id string) (model.URLModel, error) {
	if id == fmt.Sprintf("%08d", m.counter-1) && m.counter > 0 && len(m.urls) > 0 {
		return m.urls[m.counter-1], nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}
