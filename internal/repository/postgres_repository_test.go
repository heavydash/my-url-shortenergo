package repository

import (
	"context"
	"errors"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestPostgresRepo(t *testing.T) *PostgresRepository {
	t.Helper()

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN environment variable not set")
	}

	// Создаём минимальный тестовый конфиг`
	testCfg := &config.Config{
		PingTimeout:         5 * time.Second, // или любое разумное значение
		DBMaxConns:          5,
		DBMinConns:          1,
		DBMaxConnLifetime:   5 * time.Minute,
		DBHealthCheckPeriod: 1 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.New(ctx, dsn, testCfg)
	require.NoError(t, err, "failed to connect to postgres")

	logger := zap.NewNop()

	repo := NewPostgresRepository(pool.Pool, logger, "http://localhost:8080")

	// Очистка таблиц перед каждым тестом
	_, err = pool.Exec(ctx, "TRUNCATE TABLE urls RESTART IDENTITY CASCADE")
	require.NoError(t, err, "failed to truncate urls table")

	t.Cleanup(func() {
		pool.Close()
	})
	return repo
}

func TestPostgresRepository_SaveURL(t *testing.T) {
	repo := newTestPostgresRepo(t)

	url := &model.URLModel{
		OriginalURL: "http://example.com",
	}

	saved, err := repo.SaveURL(*url)
	require.NoError(t, err, "failed to save url")

	t.Logf("Saved URL: %+v", saved)

	// Проверки
	assert.NotEqual(t, uuid.Nil, saved.ID, "ID should be generated")
	assert.Equal(t, url.OriginalURL, saved.OriginalURL, "OriginalURL should match")
	assert.False(t, saved.IsDeleted, "IsDeleted should be false")

	if saved.ShortURL == "" {
		t.Log("Note: ShortURL is empty")
	}
}

func TestPostgresRepository_SaveURL_WithShortURL(t *testing.T) {
	repo := newTestPostgresRepo(t)

	shortURL := "abc123"
	url := model.URLModel{
		OriginalURL: "http://example.com",
		ShortURL:    shortURL,
	}

	saved, err := repo.SaveURL(url)
	require.NoError(t, err, "failed to save url")

	t.Logf("Saved URL: %+v", saved)

	// Проверяем, что short_url сохранился
	assert.Equal(t, shortURL, saved.ShortURL, "ShortURL should be as provided")
	assert.NotEqual(t, uuid.Nil, saved.ID, "ID should be generated")
}

func TestPostgresRepository_SaveURL_Conflict(t *testing.T) {
	repo := newTestPostgresRepo(t)

	url := &model.URLModel{
		OriginalURL: "http://example.com",
	}

	// Первое сохранение
	firstSave, err := repo.SaveURL(*url)
	require.NoError(t, err, "failed to save url")

	// Повторное сохранение
	secondSave, err := repo.SaveURL(*url)

	// Проверяем, что именно ErrConflict
	require.Error(t, err, "expected conflict error")

	var repoErr error
	if errors.Is(err, ErrConflict) {
		repoErr = ErrConflict
	} else if strings.Contains(err.Error(), "already exists") {
		t.Logf("Got conflict error (not ErrConflict type): %v", err)
	} else {
		t.Fatalf("Expected conflict error, got: %v", err)
	}

	// Проверяем, что вернулась существующая запись
	if secondSave.ID != uuid.Nil {
		assert.Equal(t, firstSave.ID, secondSave.ID, "should return existing id")
		assert.Equal(t, firstSave.OriginalURL, secondSave.OriginalURL, "should return existing original URL")
	}

	t.Logf("Conflict handled: err=%v, returnedID=%v", repoErr, secondSave.ID)
}
func TestPostgresRepository_GetURL_Found(t *testing.T) {
	repo := newTestPostgresRepo(t)

	shortURL := "abc123"
	url := &model.URLModel{
		OriginalURL: "http://example.com",
		ShortURL:    shortURL,
	}

	saved, err := repo.SaveURL(*url)
	require.NoError(t, err, "failed to save url")

	// Убедимся, что short_url сохранился
	require.NotEmpty(t, saved.ShortURL, "ShortURL should not be empty for this test")

	// Ищем по short_url
	got, err := repo.GetURL(saved.ShortURL)
	require.NoError(t, err, "GetURL failed")

	assert.Equal(t, saved.ID, got.ID, "should return existing ID")
	assert.Equal(t, saved.ShortURL, got.ShortURL, "should return existing ShortURL")
	assert.Equal(t, saved.OriginalURL, got.OriginalURL, "should return existing OriginalURL")
	assert.False(t, got.IsDeleted, "should not be deleted")
}

func TestPostgresRepository_GetURL_NotFound(t *testing.T) {
	repo := newTestPostgresRepo(t)

	_, err := repo.GetURL("nonexistent-uuid")
	require.Error(t, err, "expected error or not found")

	assert.Contains(t, strings.ToLower(err.Error()), "not found",
		"error should contain 'not found'")
}

func TestPostgresRepository_GetURL_Deleted(t *testing.T) {
	repo := newTestPostgresRepo(t)

	// Вставляем запись напрямую с ShortURL
	ctx := context.Background()
	id := uuid.New()
	shortURL := "deleted-test-789"

	_, err := repo.pool.Exec(ctx,
		"INSERT INTO urls (id, short_url, original_url, user_id, is_deleted) VALUES ($1, $2, $3, $4, $5)",
		id, shortURL, "http://deleted.com", uuid.Nil, false)
	require.NoError(t, err)

	// Помечаем как удаленную
	_, err = repo.pool.Exec(ctx,
		"UPDATE urls SET is_deleted = true WHERE short_url = $1",
		shortURL)
	require.NoError(t, err)

	// Получаем через GetURL
	got, err := repo.GetURL(shortURL)
	require.NoError(t, err, "GetURL should still work for deleted URLs")
	assert.True(t, got.IsDeleted, "should be marked as deleted")
	assert.Equal(t, id, got.ID, "ID should match")
}
