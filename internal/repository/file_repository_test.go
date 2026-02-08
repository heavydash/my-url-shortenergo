package repository

import (
	"github.com/google/uuid"
	"os"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRepository(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "urls_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	repo := NewFileRepository(tmpFile.Name())
	require.NoError(t, err)

	url := model.URLModel{
		OriginalURL: "http://example.com",
		UserID:      uuid.New(),
	}

	// Save
	saved, err := repo.SaveURL(url)
	require.NoError(t, err)

	// Проверки
	assert.NotEmpty(t, saved.UUID)
	assert.NotEmpty(t, saved.ShortURL)
	assert.Equal(t, saved.UUID, saved.ShortURL)
	assert.Equal(t, url.OriginalURL, saved.OriginalURL)
	assert.Equal(t, url.UserID, saved.UserID)
	assert.False(t, saved.IsDeleted)
}

func TestFileRepository_GetURL_Found(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_repo_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	repo := NewFileRepository(tmpFile.Name())
	require.NoError(t, err)

	// Сначала сохраняем
	url := model.URLModel{
		OriginalURL: "http://example.com",
		ShortURL:    "test123",
		UserID:      uuid.New(),
	}

	saved, err := repo.SaveURL(url)
	require.NoError(t, err)

	// Потом получаем
	got, err := repo.GetURL(saved.ShortURL)
	require.NoError(t, err)
	assert.Equal(t, saved.UUID, got.UUID) // Используем UUID вместо ID
	assert.Equal(t, saved.OriginalURL, got.OriginalURL)
	assert.Equal(t, saved.ShortURL, got.ShortURL)
	assert.Equal(t, saved.UserID, got.UserID)
}

func TestFileRepository_GetURL_NotFound(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_repo_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	repo := NewFileRepository(tmpFile.Name())
	require.NoError(t, err)
	// defer repo.Close() // Убрали

	_, err = repo.GetURL("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
