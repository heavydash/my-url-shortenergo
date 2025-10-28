package repository

import (
	"encoding/json"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestFileRepository_NewFileRepository(t *testing.T) {
	t.Run("Create with empty file", func(t *testing.T) {
		tmp, err := os.CreateTemp("", "test_urls.json")
		require.NoError(t, err)
		defer os.Remove(tmp.Name())

		repo, err := NewFileRepository(tmp.Name())
		assert.Empty(t, repo.urls)

	})
	t.Run("Create with existing file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test_urls*.json")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		urls := []model.URLModel{
			{UUID: "testid", ShortURL: "testid", OriginalURL: "http://example.com"},
		}
		data, err := json.Marshal(urls)
		require.NoError(t, err)
		err = os.WriteFile(tmpFile.Name(), data, 0644)
		repo, err := NewFileRepository(tmpFile.Name())
		require.NoError(t, err)

		assert.Len(t, repo.urls, 1)
		assert.Equal(t, repo.urls["testid"].OriginalURL, "http://example.com")
	})
}

func TestFileRepository_SaveURL(t *testing.T) {
	t.Run("Save new url", func(t *testing.T) {
		tmp, err := os.CreateTemp("", "test_urls.json")
		require.NoError(t, err)
		defer os.Remove(tmp.Name())

		repo, err := NewFileRepository(tmp.Name())
		require.NoError(t, err)

		input := model.URLModel{UUID: "dup", OriginalURL: "http://example.com"}
		saved, err := repo.SaveURL(input)
		require.NoError(t, err)

		require.NotEmpty(t, saved.UUID)
		assert.Contains(t, repo.urls, saved.UUID)

		data, err := os.ReadFile(tmp.Name())
		require.NoError(t, err)
		assert.Contains(t, string(data), input.OriginalURL)
	})
	t.Run("Duplicate UUID", func(t *testing.T) {
		tmp, err := os.CreateTemp("", "test_urls.json")
		require.NoError(t, err)
		defer os.Remove(tmp.Name())

		repo, err := NewFileRepository(tmp.Name())
		require.NoError(t, err)

		model := model.URLModel{
			UUID:        "dup",
			OriginalURL: "http://example.com"}

		_, err = repo.SaveURL(model)
		require.NoError(t, err)

		_, err = repo.SaveURL(model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}
func TestFileRepository_GetURL(t *testing.T) {
	t.Run("Get existing url", func(t *testing.T) {
		tmp, _ := os.CreateTemp("", "test_urls.json")
		require.NotNil(t, tmp)
		defer os.Remove(tmp.Name())

		repo, err := NewFileRepository(tmp.Name())
		require.NoError(t, err)

		input := model.URLModel{OriginalURL: "http://example.com"}
		saved, err := repo.SaveURL(input)
		require.NoError(t, err)

		got, err := repo.GetURL(saved.UUID)
		require.NoError(t, err)
		assert.Equal(t, saved, got)

	})
	t.Run("Get non existing url", func(t *testing.T) {
		tmp, _ := os.CreateTemp("", "test_urls.json")
		require.NotNil(t, tmp)
		defer os.Remove(tmp.Name())

		repo, err := NewFileRepository(tmp.Name())
		require.NoError(t, err)

		_, err = repo.GetURL("non-existing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
func TestFileRepository_Clear(t *testing.T) {
	tmp, _ := os.CreateTemp("", "test_urls_*.json")
	require.NotNil(t, tmp)
	defer os.Remove(tmp.Name())

	repo, err := NewFileRepository(tmp.Name())
	require.NoError(t, err)

	_, err = repo.SaveURL(model.URLModel{OriginalURL: "http://example.com"})
	require.NoError(t, err)

	repo.Clear()

	assert.Empty(t, repo.urls)

	data, err := os.ReadFile(tmp.Name())
	require.NoError(t, err)
	assert.Empty(t, string(data))
}
