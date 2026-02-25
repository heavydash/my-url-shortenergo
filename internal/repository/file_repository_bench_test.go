package repository

import (
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Бенчмарки
func newTempFilerepo(b *testing.B) *FileRepository {
	b.Helper()

	tmpDir := b.TempDir() // создает временную директорию для теста
	tmpFile := filepath.Join(tmpDir, "urls.json")

	repo := NewFileRepository(tmpFile)

	// Удаляем файл после завершения бенчмарка
	b.Cleanup(func() {
		os.Remove(tmpFile)
	})
	return repo
}

func BenchmarkFileRepo_Save(b *testing.B) {
	b.Log("STARTING FILE REPO SAVE")
	repo := newTempFilerepo(b)

	url := model.URLModel{
		OriginalURL: "http://example.com",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.SaveURL(url)
	}
}

func BenchmarkFileRepo_GetURL_Found(b *testing.B) {
	repo := newTempFilerepo(b)

	savedKeys := make([]string, 200)
	for i := 0; i < 200; i++ {
		url := model.URLModel{
			OriginalURL: "http://example.com" + strconv.Itoa(i),
		}
		saved, _ := repo.SaveURL(url)
		savedKeys[i] = saved.UUID
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetURL(savedKeys[i%200])
	}
}

func BenchmarkFileRepo_GetURL_NotFound(b *testing.B) {
	repo := newTempFilerepo(b)

	for i := 0; i < 200; i++ {
		repo.SaveURL(model.URLModel{
			OriginalURL: "http://example.com",
		})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetURL("nonexistent" + strconv.Itoa(i))
	}
}
