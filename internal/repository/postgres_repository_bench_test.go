package repository

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"go.uber.org/zap"
	"os"
	"testing"
	"time"
)

// Бенчмарки

func newBenchmarkRepo(b *testing.B) *PostgresRepository {
	b.Helper()

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		b.Skip("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.New(ctx, dsn)
	if err != nil {
		b.Fatalf("failed to connect postgres: %v", err)
	}

	logger := zap.NewNop()
	repo := &PostgresRepository{pool.Pool, logger}

	// Очистка таблиц перед бенчмарком
	_, err = pool.Exec(ctx, "TRUNCATE TABLE urls RESTART IDENTITY CASCADE")
	if err != nil {
		b.Fatalf("failed to truncate urls table: %v", err)
	}

	b.Cleanup(func() {
		pool.Close()
	})
	return repo
}

func BenchmarkPostgresRepo_Save(b *testing.B) {
	repo := newBenchmarkRepo(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := model.URLModel{
			OriginalURL: fmt.Sprintf("http://example.com/%d", i),
			ShortURL:    fmt.Sprintf("s%d", i),
		}

		_, err := repo.SaveURL(url)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPostgresRepo_GetURL(b *testing.B) {
	repo := newBenchmarkRepo(b)
	ctx := context.Background()

	// Тестовые данные
	letters := "abcdefghijklmnopqrstuvwxyz"
	for i := 0; i < 26; i++ {
		shortURL := string(letters[i])

		// Вставляем напрямую
		id := uuid.New()
		_, err := repo.pool.Exec(ctx,
			"INSERT INTO urls (id, short_url, original_url, user_id, is_deleted) VALUES ($1, $2, $3, $4, $5)",
			id, shortURL, "http://example.com/"+shortURL, uuid.Nil, false)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		letterIndex := i % 26
		shortURL := string(letters[letterIndex])
		_, err := repo.GetURL(shortURL)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPostgresRepo_Conflict(b *testing.B) {
	repo := newBenchmarkRepo(b)

	baseURL := model.URLModel{
		OriginalURL: "http://example.com/",
		ShortURL:    "base",
	}

	_, err := repo.SaveURL(baseURL)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	// Пытаемся сохранить URL много раз
	for i := 0; i < b.N; i++ {
		confictURL := model.URLModel{
			OriginalURL: "http://example.com/",
			ShortURL:    fmt.Sprintf("conflict%d", i),
		}

		_, err := repo.SaveURL(confictURL)
		if err != nil && !isConflictError(err) {
			b.Fatal(err)
		}

	}
}

func isConflictError(err error) bool {
	return err != nil && (err.Error() == "url already exists")
}
