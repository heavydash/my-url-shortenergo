package handler

import (
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
		cfg := &config.Config{BaseURL: "https://test.com/"}
		h := NewHandler(repo, cfg)
		r := chi.NewRouter()
		h.SetupRoutes(r)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", strings.NewReader("https://example.com"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "https://test.com/")
	})
}

func TestHandler_HomeHandler(t *testing.T) {
	t.Run("Valid GET", func(t *testing.T) {
		repo := repository.NewMemoryRepository()
		cfg := &config.Config{}
		h := NewHandler(repo, cfg)
		r := chi.NewRouter()
		h.SetupRoutes(r)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "URL Shortener Service")
	})
}
