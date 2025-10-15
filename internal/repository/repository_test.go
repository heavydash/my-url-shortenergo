package repository

import (
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMemoryRepository_SaveURL(t *testing.T) {
	repo := NewMemoryRepository()
	repo.Clear()

	tests := []struct {
		name    string
		model   model.URLModel
		wantErr bool
	}{
		{"save new URL", model.URLModel{URL: "http://example.com"}, false},
		{"save empty URL", model.URLModel{URL: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, err := repo.SaveURL(tt.model)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, saved.ID)
			assert.Equal(t, tt.model.URL, saved.URL)

			got, err := repo.GetURL(saved.ID)
			require.NoError(t, err)
			assert.Equal(t, saved.ID, got.ID)
		})
	}
}
func TestMemoryRepository_GetURL_NotFound(t *testing.T) {
	repo := NewMemoryRepository()
	repo.Clear()

	_, err := repo.GetURL("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMemoryRepository_SaveURL_ExistingID(t *testing.T) {
	repo := NewMemoryRepository()
	repo.Clear()

	modelanother := model.URLModel{ID: "testid", URL: "http://example.com"}
	_, err := repo.SaveURL(modelanother)
	require.NoError(t, err)

	_, err = repo.SaveURL(modelanother)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMemoryRepository_Clear(t *testing.T) {
	repo := NewMemoryRepository()
	saved, _ := repo.SaveURL(model.URLModel{URL: "http://example.com"})
	repo.Clear()

	_, err := repo.GetURL(saved.ID)
	require.NoError(t, err)
}
