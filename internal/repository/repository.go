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
	urls    map[string]model.URLModel
	counter int
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		urls:    make(map[string]model.URLModel),
		counter: 0,
	}
}

func (mr *MemoryRepository) InitializeForTest(id string, url string) {
	mr.urls[id] = model.URLModel{ID: id, URL: url}
	mr.counter = 1
}

func (mr *MemoryRepository) SaveURL(m model.URLModel) (model.URLModel, error) {
	id := fmt.Sprintf("%08d", mr.counter)
	mr.counter++
	if _, exists := mr.urls[id]; exists {
		mr.counter++
		id = fmt.Sprintf("%08d", mr.counter)
	}
	NewModel := model.URLModel{ID: id, URL: m.URL}
	mr.urls[id] = NewModel
	return NewModel, nil
}
func (mr *MemoryRepository) GetURL(id string) (model.URLModel, error) {
	if m, exists := mr.urls[id]; exists {
		return m, nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}
