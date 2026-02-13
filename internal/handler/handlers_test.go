package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/internal/repository/mocks"
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

	auditNoop := &service.Noop{}

	return NewHandler(
		repo,
		cfg,
		logger,
		auditNoop,
	)
}

func setupTest(t *testing.T) (*chi.Mux, *httptest.ResponseRecorder, *config.Config, *repository.MemoryRepository) {
	repo := repository.NewMemoryRepository("http://localhost:8080")
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	cfg := &config.Config{BaseURL: "http://localhost:33675/"}

	logger, _ := zap.NewProduction()
	t.Cleanup(func() { _ = logger.Sync() })

	h := newTestHandler(t, repo, cfg, logger)

	router := chi.NewRouter()

	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware)

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
	router.Use(middleware.GzipMiddleware)

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

func TestHandler_ShortenURL(t *testing.T) {
	t.Run("Valid POST", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
		req := httptest.NewRequest("POST", "/", strings.NewReader("https://example.com"))

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		body := getResponseBody(t, w)
		assert.Contains(t, body, "http://localhost:33675/")
	})
}

func TestHandler_HomeHandler(t *testing.T) {
	t.Run("Valid GET", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
		req := httptest.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		body := getResponseBody(t, w)
		assert.Contains(t, body, "URL Shortener Service")
		assert.Contains(t, body, "POST /")
		assert.Contains(t, body, "GET /{id}")
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

// handlers_plain_test.go :
func TestRedirectURLSuccess(t *testing.T) {
	ctrl := gomock.NewController(t) // создаем контроллер
	defer ctrl.Finish()

	mockRepo := mocks.NewMockURLRepository(ctrl) // создаем мок

	// Что должен получить вызов и что ответить
	mockRepo.EXPECT().
		GetURL("goodid").
		Return(model.URLModel{OriginalURL: "http:/ya.ru"}, nil)

	// Запускаем хендлер с мок репозиторием
	h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"},
		zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/goodid", nil)
	w := httptest.NewRecorder()

	h.RedirectURL(w, req)

	// gomock проверяет был ли вызов GetURL ("test")
	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "http:/ya.ru", w.Header().Get("Location"))
}

func TestRedirectURLNotFound(t *testing.T) {
	ctrl := gomock.NewController(t) // создаем контроллер
	defer ctrl.Finish()
	mockRepo := mocks.NewMockURLRepository(ctrl) // создаем мок

	// Что должен получить вызов и что ответить
	mockRepo.EXPECT().
		GetURL("goodid").
		Return(model.URLModel{}, errors.New("not found"))

	// Запускаем хендлер с мок репозиторием
	h := newTestHandler(t, mockRepo, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/goodid", nil)
	w := httptest.NewRecorder()

	h.RedirectURL(w, req)

	// gomock проверяет был ли вызов GetURL ("test")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPingHandlerOK(t *testing.T) {
	ctrl := gomock.NewController(t) // создаем контроллер
	defer ctrl.Finish()
	repo := mocks.NewMockURLRepository(ctrl)     // создаем мок
	repo.EXPECT().Ping(gomock.Any()).Return(nil) // Что должен получить вызов и что ответить

	// Запускаем хендлер с мок репозиторием
	h := newTestHandler(t, repo, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	h.PingHandler(w, req)

	// gomock проверяет был ли вызов GetURL ("test")
	assert.Equal(t, http.StatusOK, w.Code)
}

// handlers_api_test.go :

func TestShortenURL_JSON(t *testing.T) {
	t.Run("Valid POST JSON", func(t *testing.T) {
		r, w, _, _ := setupTest(t)
		req := httptest.NewRequest("POST", "/api/shorten", strings.NewReader("{\"url\":\"https://example.com\"}"))

		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		body := getResponseBody(t, w)
		assert.Contains(t, body, "http://localhost:33675/")
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

func TestGetUserURLs_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockURLRepository(ctrl)
	mockRepo.EXPECT().
		GetURLsByUser(gomock.Any(), gomock.Any()).
		Return([]model.URLModel{}, nil)

	h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())

	router, rec := SetupTestRouter(t, h)

	req := httptest.NewRequest("GET", "/api/user/urls", nil)
	req.AddCookie(&http.Cookie{Name: "user_id", Value: "test"})

	router.ServeHTTP(&rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

// Бенчмарки
// Создание новой короткой ссылки
func BenchmarkShorten_NewURL(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	logger := zap.NewNop()
	auditNoop := service.Noop{}

	h := NewHandler(repo, cfg, logger, auditNoop)

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.ShortenHandler(w, req, false)
	}
}

// Повторное сокращение уже существующей ссылки
func BenchmarkShorten_ExistingURL(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &service.Noop{}

	h := NewHandler(repo, cfg, logger, auditNoop)

	body := strings.NewReader("https://example.com")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ShortenHandler(w, req, false)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.ShortenHandler(w, req, false)
	}
}

// Редирект
func BenchmarkResolve_Found(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &service.Noop{}

	h := NewHandler(repo, cfg, logger, auditNoop)

	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.RedirectURL(w, req)
	}
}

// Несуществующий короткий код
func BenchmarkResolve_NotFound(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &service.Noop{}

	h := NewHandler(repo, cfg, logger, auditNoop)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent123", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.RedirectURL(w, req)
	}
}

// Batch
func BenchmarkBatchShorten(b *testing.B) {
	//start
	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &service.Noop{}

	h := NewHandler(repo, cfg, logger, auditNoop)

	body := strings.NewReader(`[
		{"correlation_id": "1", "original_url": "https://ya.ru"},
		{"correlation_id": "2", "original_url": "https://google.com"},
		{"correlation_id": "3", "original_url": "https://example.com"},
	]`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.BatchShortenHandler(w, req)
	}
}

func getResponseBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	bodyBytes := w.Body.Bytes()

	if w.Header().Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		require.NoError(t, err, "failed to create gzip reader")
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err, "failed to decompress gzip reader")
		bodyBytes = decompressed
	}
	return string(bodyBytes)
}
