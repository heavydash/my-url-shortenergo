package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPostgresRepository(t *testing.T) {
	// Поднимаем тестовую БД
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.New(ctx, "postgres://praktikum:12345@localhost:8080/praktikum?sslmode=disable")
	require.NoError(t, err)
	defer pool.Close()

	repo := NewPostgresRepository(pool.Pool)

	// Очистка
	_, err = pool.Exec(ctx, "TRUNCATE TABLE urls RESTART IDENTITY")
	require.NoError(t, err)

	userID := uuid.New()

	t.Run("Save and Get URL", func(t *testing.T) {
		urlModel := model.URLModel{
			UUID:        "test1",
			ShortURL:    "test1",
			OriginalURL: "http://test1.example.com",
			UserID:      userID,
		}

		saved, err := repo.SaveURL(urlModel)
		require.NoError(t, err)
		assert.Contains(t, saved.ShortURL, "test1")

		got, err := repo.GetURL("test1")
		require.NoError(t, err)
		assert.Equal(t, "http://test1.example.com", got.OriginalURL)
	})

	t.Run("GetURLsByUser", func(t *testing.T) {
		urls, err := repo.GetURLsByUser(ctx, userID)
		require.NoError(t, err)
		require.Len(t, urls, 1)
		assert.Contains(t, urls[0].ShortURL, "test1")
		assert.Equal(t, "http://test1.example.com", urls[0].OriginalURL)
	})

	t.Run("Not found", func(t *testing.T) {
		_, err := repo.GetURL("not exists")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
