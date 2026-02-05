package repository

import (
	"strconv"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRepository_SaveURL(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	tests := []struct {
		name    string
		model   model.URLModel
		wantErr bool
	}{
		{"save new URL", model.URLModel{OriginalURL: "http://example.com"}, false},
		{"save empty URL", model.URLModel{OriginalURL: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, err := repo.SaveURL(tt.model)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, saved.UUID)
			assert.Equal(t, tt.model.OriginalURL, saved.OriginalURL)

			got, err := repo.GetURL(saved.UUID)
			require.NoError(t, err)
			assert.Equal(t, saved.UUID, got.UUID)
		})
	}
}
func TestMemoryRepository_GetURL_NotFound(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	_, err := repo.GetURL("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMemoryRepository_SaveURL_ExistingID(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	modelanother := model.URLModel{UUID: "testid", OriginalURL: "http://example.com"}
	_, err := repo.SaveURL(modelanother)
	require.NoError(t, err)

	_, err = repo.SaveURL(modelanother)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMemoryRepository_Clear(t *testing.T) {
	repo := NewMemoryRepository()
	model1 := model.URLModel{OriginalURL: "http://example.com"}
	savedModel, err := repo.SaveURL(model1)
	require.NoError(t, err)
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	_, err = repo.GetURL(savedModel.UUID)
	assert.Error(t, err)
}

// Бенчмарки
// SaveURL
func BenchmarkRepo_Save(b *testing.B) {
	repo := NewMemoryRepository()
	original := model.URLModel{
		OriginalURL: "http://example.com",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Создаем копию модели каждую итерацию
		url := original
		_, _ = repo.SaveURL(url)
	}
}

// Found
func BenchmarkRepo_GetURLFound(b *testing.B) {
	repo := NewMemoryRepository()

	savedKeys := make([]string, 1000)

	// Предзаполнение
	for i := 0; i < 1000; i++ {
		url := model.URLModel{
			OriginalURL: "http://example.com/" + strconv.Itoa(i)}
		saved, _ := repo.SaveURL(url)
		savedKeys[i] = saved.UUID // сохраняем настоящий ключ
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Случайный существующий ключ
		key := savedKeys[i%1000]
		_, _ = repo.GetURL(key)
	}
}

// NotFound
func BenchmarkRepo_GetURLNotFound(b *testing.B) {
	repo := NewMemoryRepository()

	// Предзаполняем
	for i := 0; i < 1000; i++ {
		repo.SaveURL(model.URLModel{OriginalURL: "http://example.com/"})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetURL("nonexistent" + strconv.Itoa(i))
	}
}
