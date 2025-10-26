package handler

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (*chi.Mux, *httptest.ResponseRecorder, *config.Config, *repository.MemoryRepository) {
	repo := repository.NewMemoryRepository()
	repo.Clear()
	cfg := &config.Config{BaseURL: "http://localhost:33675/"}
	h := NewHandler(repo, cfg)
	r := chi.NewRouter()
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)
	w := httptest.NewRecorder()
	return r, w, cfg, repo
}

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
func TestGzipIn(t *testing.T) {
	t.Run("GZIP POST JSON Incoming", func(t *testing.T) {
		r, w, _, _ := setupTest(t)

		buf := &bytes.Buffer{}
		gw := gzip.NewWriter(buf)
		_, err := gw.Write([]byte(`{"url":"https://example.com"}`))
		assert.NoError(t, err)
		err = gw.Close()
		assert.NoError(t, err)

		req := httptest.NewRequest("POST", "/api/shorten", buf)
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "http://localhost:33675/")

	})
}
func TestGzipOut(t *testing.T) {
	t.Run("GZIP POST JSON Outcome", func(t *testing.T) {
		r, w, _, _ := setupTest(t)

		req := httptest.NewRequest("POST", "/api/shorten", strings.NewReader(`{"url":"https://example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

		gr, err := gzip.NewReader(w.Body)
		assert.NoError(t, err)
		body, err := io.ReadAll(gr)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "http://localhost:33675/")
	})
}
