package handler

import (
	"bytes"
	"compress/gzip"
	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"go.uber.org/zap"
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
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	cfg := &config.Config{BaseURL: "http://localhost:33675/"}

	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	h := NewHandler(repo, cfg, logger)
	r := chi.NewRouter()
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)
	w := httptest.NewRecorder()
	return r, w, cfg, repo
}

func TestHandler_ShortenURL(t *testing.T) {
	t.Run("Valid POST", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
		req := httptest.NewRequest("POST", "/", strings.NewReader("https://example.com"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "http://localhost:33675/")
	})
}

func TestHandler_HomeHandler(t *testing.T) {
	t.Run("Valid GET", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
		req := httptest.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "URL Shortener Service")
		assert.Contains(t, w.Body.String(), "POST /")
		assert.Contains(t, w.Body.String(), "GET /{id}")
	})
}
func TestShortenURL_JSON(t *testing.T) {
	t.Run("Valid POST JSON", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
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
