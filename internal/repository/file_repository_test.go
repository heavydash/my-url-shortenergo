package repository

import (
	"context"
	"os"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRepository(t *testing.T) {
	tmp, _ := os.CreateTemp("", "urls_*.json")
	defer os.Remove(tmp.Name())

	// Save
	repo := NewFileRepository(tmp.Name())
	saved, err := repo.SaveURL(context.Background(), model.URLModel{OriginalURL: "http://example.com"})
	require.NoError(t, err)
	assert.NotEmpty(t, saved.UUID)

	// Load from file
	repo2 := NewFileRepository(tmp.Name())
	got, err := repo2.GetURL(context.Background(), saved.UUID)
	require.NoError(t, err)
	assert.Equal(t, "http://example.com", got.OriginalURL)

	// Clear
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	_, err = repo.GetURL(context.Background(), saved.UUID)
	assert.ErrorContains(t, err, "not found")
}
