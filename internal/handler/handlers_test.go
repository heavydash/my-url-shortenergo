package handler

import (
	"fmt"
	"github.com/go-chi/chi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/stretchr/testify/assert"
)

func TestHandler_ShortenURL(t *testing.T) {
	t.Run("Valid POST", func(t *testing.T) {
		repo := repository.NewMemoryRepository()
		repo.Clear()
		cfg := &config.Config{BaseURL: fmt.Sprintf("http://localhost:%d/", 33675)}
		h := NewHandler(repo, cfg)
		r := chi.NewRouter()
		h.SetupRoutes(r)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", strings.NewReader("https://example.com"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "http://localhost:33675/")
	})
}

func TestHandler_HomeHandler(t *testing.T) {
	t.Run("Valid GET", func(t *testing.T) {
		repo := repository.NewMemoryRepository()
		repo.Clear()
		cfg := &config.Config{}
		h := NewHandler(repo, cfg)
		r := chi.NewRouter()
		h.SetupRoutes(r)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), cfg.BaseURL)
	})
}
func TestShortenURL_JSON(t *testing.T) {
	t.Run("Valid POST JSON", func(t *testing.T) {
		repo := repository.NewMemoryRepository()
		repo.Clear()
		cfg := &config.Config{BaseURL: fmt.Sprintf("http://localhost:%d/", 33675)}
		h := NewHandler(repo, cfg)
		r := chi.NewRouter()
		h.SetupRoutes(r)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/shorten", strings.NewReader("{\"url\":\"https://example.com\"}"))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "http://localhost:33675/")
	})
}
