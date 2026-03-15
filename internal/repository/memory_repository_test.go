package repository

import (
	"go.uber.org/zap"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRepository_SaveURL(t *testing.T) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())
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
			saved, err := repo.SaveURL(t.Context(), tt.model)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, saved.UUID)
			assert.Equal(t, tt.model.OriginalURL, saved.OriginalURL)

			got, err := repo.GetURL(t.Context(), saved.UUID)
			require.NoError(t, err)
			assert.Equal(t, saved.UUID, got.UUID)
		})
	}
}
func TestMemoryRepository_GetURL_NotFound(t *testing.T) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	_, err := repo.GetURL(t.Context(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMemoryRepository_SaveURL_ExistingID(t *testing.T) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	modelanother := model.URLModel{UUID: "testid", OriginalURL: "http://example.com"}
	_, err := repo.SaveURL(t.Context(), modelanother)
	require.NoError(t, err)

	_, err = repo.SaveURL(t.Context(), modelanother)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMemoryRepository_Clear(t *testing.T) {
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())
	model1 := model.URLModel{OriginalURL: "http://example.com"}
	savedModel, saveErr := repo.SaveURL(t.Context(), model1)
	require.NoError(t, saveErr)

	clearErr := repo.Clear()
	if clearErr != nil {
		t.Fatalf("Clear failed: %v", clearErr)
	}

	_, getErr := repo.GetURL(t.Context(), savedModel.UUID)
	assert.Error(t, getErr)
}
