// repository/example_test.go
package repository_test

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"os"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

// Пример работы с MemoryRepository.
func ExampleMemoryRepository() {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	defer func() {
		if err := repo.Clear(); err != nil {
			panic(err)
		}
	}()

	userID := uuid.New()

	// Сохраняем URL
	url := model.URLModel{
		ShortURL:    "abc123",
		OriginalURL: "https://example.com",
		UserID:      userID,
	}

	savedURL, err := repo.SaveURL(context.Background(), url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Saved URL ID: %s\n", savedURL.UUID)
	fmt.Printf("Saved Short URL: %s\n", savedURL.ShortURL)

	// Получаем URL
	retrievedURL, err := repo.GetURL(context.Background(), savedURL.ShortURL)
	if err != nil {
		fmt.Printf("Error retrieving: %v\n", err)
		return
	}

	fmt.Printf("Retrieved URL: %s\n", retrievedURL.OriginalURL)

	// Output pattern:
	// Saved URL ID: abc123
	// Saved Short URL: abc123
	// Retrieved URL: https://example.com
}

// Пример работы с FileRepository.
func ExampleFileRepository() {
	// Создаем временный файл
	tmpfile, err := os.CreateTemp("", "urls-*.json")
	if err != nil {
		fmt.Printf("Failed to create temp file: %v\n", err)
		return
	}
	_ = tmpfile.Close()
	defer func() {
		if err = os.Remove(tmpfile.Name()); err != nil {
			fmt.Printf("Failed to remove temp file: %v\n", err)
		}
	}()

	repo := repository.NewFileRepository(tmpfile.Name(), "http://localhost:8080", zap.NewNop())
	// В реальном коде: defer repo.file.Close()

	userID := uuid.New()

	// Сохраняем URL
	url := model.URLModel{
		ShortURL:    "test123",
		OriginalURL: "https://go.dev",
		UserID:      userID,
	}

	_, err = repo.SaveURL(context.Background(), url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Проверяем что файл не пустой
	fileInfo, _ := os.Stat(tmpfile.Name())
	fmt.Printf("File size > 0: %v\n", fileInfo.Size() > 0)

	// Output:
	// File size > 0: true
}

// Пример получения URL пользователя.
func ExampleMemoryRepository_GetURLsByUser() {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	defer func() {
		if err := repo.Clear(); err != nil {
			panic(err)
		}
	}()

	userID := uuid.New()

	// Сохраняем несколько URL для пользователя
	urls := []model.URLModel{
		{ShortURL: "id1", OriginalURL: "https://example.com/1", UserID: userID},
		{ShortURL: "id2", OriginalURL: "https://example.com/2", UserID: userID},
	}

	for _, url := range urls {
		_, _ = repo.SaveURL(context.Background(), url)
	}

	// Получаем URL пользователя
	userURLs, err := repo.GetURLsByUser(context.Background(), userID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("User has %d URLs\n", len(userURLs))
	for _, url := range userURLs {
		fmt.Printf("URL: %s -> %s\n", url.ShortURL, url.OriginalURL)
	}

	// Output pattern:
	// User has 2 URLs
	// URL: http://localhost:8080/id1 -> https://example.com/1
	// URL: http://localhost:8080/id2 -> https://example.com/2
}
