package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

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
	t.Cleanup(func() { _ = logger.Sync() })

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
func TestBatchShortenHandler_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockURLRepository(ctrl)
	_ = mockRepo
	mockRepo.EXPECT().
		SaveBatch(gomock.Any(), gomock.Len(2)).
		Return(nil).
		Times(1)

	h := NewHandler(mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, nil)

	body := `[
		{"correlation_id": "1", "original_url": "https://ya.ru"},
		{"correlation_id": "2", "original_url": "https://google.com"}
	]`

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.BatchShortenHandler(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp []model.BatchResponseItem
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp, 2)
	require.Equal(t, "1", resp[0].CorrelationID)
	require.Contains(t, resp[0].OriginalURL, "http://localhost:8080/")
	require.Equal(t, "2", resp[1].CorrelationID)
	require.Contains(t, resp[1].OriginalURL, "http://localhost:8080/")
}

func TestBatchShortenHandler_EmptyBatch(t *testing.T) {
	h := NewHandler(nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader([]byte(`[]`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.BatchShortenHandler(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "empty batch")
}

func TestBatchShortenHandler_DuplicateCorrelationID(t *testing.T) {
	h := NewHandler(nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

	body := `[
		{"correlation_id": "dup", "original_url": "https://ya.ru"},
		{"correlation_id": "dup", "original_url": "https://google.com"}
	]`

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.BatchShortenHandler(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "duplicate correlation_id")
}

func TestBatchShortenHandler_InvalidURL(t *testing.T) {
	h := NewHandler(nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

	body := `[
		{"correlation_id": "1", "original_url": "not-a-url"}
	]`

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.BatchShortenHandler(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid url")
}
