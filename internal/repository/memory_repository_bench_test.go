package repository

import (
	"go.uber.org/zap"
	"strconv"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/model"
)

// Бенчмарки
// SaveURL
func BenchmarkRepo_Save(b *testing.B) {
	b.Log("STARTING MEMORY REPO SAVE")
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())
	original := model.URLModel{
		OriginalURL: "http://example.com",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Создаем копию модели каждую итерацию
		url := original
		_, _ = repo.SaveURL(b.Context(), url)
	}
}

// Found
func BenchmarkRepo_GetURLFound(b *testing.B) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	savedKeys := make([]string, 1000)

	// Предзаполнение
	for i := 0; i < 1000; i++ {
		url := model.URLModel{
			OriginalURL: "http://example.com/" + strconv.Itoa(i)}
		saved, _ := repo.SaveURL(b.Context(), url)
		savedKeys[i] = saved.UUID // сохраняем настоящий ключ
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Случайный существующий ключ
		key := savedKeys[i%1000]
		_, _ = repo.GetURL(b.Context(), key)
	}
}

// NotFound
func BenchmarkRepo_GetURLNotFound(b *testing.B) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	// Предзаполняем
	for i := 0; i < 1000; i++ {
		_, _ = repo.SaveURL(b.Context(), model.URLModel{OriginalURL: "http://example.com/"})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetURL(b.Context(), "nonexistent"+strconv.Itoa(i))
	}
}
