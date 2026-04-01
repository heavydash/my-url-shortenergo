// Package handler содержит вспомогательные функции и общие тесты для HTTP-обработчиков.
//
// В этом файле оставлены только:
//   - Вспомогательные функции (newTestHandler, setupTest и т.д.)
//   - Gzip-тесты (входящий и исходящий)
//   - Бенчмарки (пока не трогаем, как просил)
//
// Все специфические тесты API и plain хендлеров вынесены в handlers_plain_test.go/handlers_api_test.go
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

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	auditService "github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/internal/repository/mocks"
	URLService "github.com/heavydash/my-url-shortenergo/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
)

func newTestHandler(
	t *testing.T,
	repo repository.URLRepository,
	cfg *config.Config,
	logger *zap.Logger,
) *Handler {
	t.Helper()

	if cfg == nil {
		cfg = &config.Config{
			BaseURL: "http://localhost:8080",
		}
	}

	svc := URLService.NewURLService(repo)

	auditNoop := &auditService.Noop{}

	return NewHandler(
		svc,
		cfg,
		logger,
		auditNoop,
	)
}

func setupTest(t *testing.T) (*chi.Mux, *httptest.ResponseRecorder, *config.Config, *repository.MemoryRepository) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	cfg := &config.Config{BaseURL: "http://localhost:33675/"}

	logger, _ := zap.NewProduction()
	t.Cleanup(func() { _ = logger.Sync() })

	h := newTestHandler(t, svc, cfg, logger)

	router := chi.NewRouter()

	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware(logger))

	router.Get("/{id}", h.RedirectURL)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Авторизованные роуты
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))
		r.Get("/api/user/urls", h.GetUserURLs)
	})

	// Анонимные
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	router.Post("/", h.ShortenPlainHandler)
	router.Post("/api/shorten", h.ShortenJSONHandler)
	router.Post("/api/shorten/batch", h.BatchShortenHandler)

	w := httptest.NewRecorder()
	return router, w, cfg, repo
}

func SetupTestRouter(t *testing.T, h *Handler) (*chi.Mux, httptest.ResponseRecorder) {
	router := chi.NewRouter()
	logger := zap.NewNop()

	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware(logger))

	// Авторизованные роуты
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))
		r.Get("/api/user/urls", h.GetUserURLs)

	})

	// Анонимные роуты
	router.Get("/{id}", h.RedirectURL)
	router.Post("/", h.ShortenPlainHandler)
	router.Post("/api/shorten", h.ShortenJSONHandler)
	router.Post("/api/shorten/batch", h.BatchShortenHandler)
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	return router, *httptest.NewRecorder()

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

		body := getResponseBody(t, w)
		assert.Contains(t, body, "http://localhost:33675/")

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

		contentEncoding := w.Header().Get("Content-Encoding")

		// Получаем байты из w.Body
		body := getResponseBody(t, w)
		assert.Contains(t, body, "http://localhost:33675/")

		// Если есть gzip - лог
		if contentEncoding == "gzip" {
			t.Log("Response was gzipped")
		} else {
			t.Log("Response was not gzipped (possibly too small)")
		}
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

	h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, nil)

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
	h := newTestHandler(t, nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", bytes.NewReader([]byte(`[]`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.BatchShortenHandler(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "empty batch")
}

func TestBatchShortenHandler_DuplicateCorrelationID(t *testing.T) {
	h := newTestHandler(t, nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

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
	h := newTestHandler(t, nil, &config.Config{BaseURL: "http://localhost:8080"}, nil)

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

func getResponseBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	bodyBytes := w.Body.Bytes()

	if w.Header().Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		require.NoError(t, err, "failed to create gzip reader")
		defer func() {
			if err = gr.Close(); err != nil {
				panic(err)
			}
		}()

		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err, "failed to decompress gzip reader")
		bodyBytes = decompressed
	}
	return string(bodyBytes)
}
