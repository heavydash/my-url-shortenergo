package repository

import (
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
