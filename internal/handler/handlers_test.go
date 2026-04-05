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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	auditService "github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
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
	t.Helper()

	// Создаем репо в памяти
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	// Очищаем репо перед тестом
	if err := repo.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Создаем сервисный слой
	svc := URLService.NewURLService(repo)

	// Настраиваем конфигурацию
	cfg := &config.Config{BaseURL: "http://localhost:33675/"}

	logger, _ := zap.NewProduction()
	t.Cleanup(func() { _ = logger.Sync() }) // сброс буфера при завершении теста

	// Создаем хендлер с зависимостями
	h := newTestHandler(t, svc, cfg, logger)
	// Создаем роутер
	router := chi.NewRouter()
	// Глобальные middleware, которе применяются ко всем запросам
	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware(logger))

	// Публичные роуты, без требований авторизации
	router.Get("/{id}", h.RedirectURL)
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Авторизованные роуты, требующие авторизации
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))

		// Основные ручки сокращения
		r.Post("/", h.ShortenPlainHandler)
		r.Post("/api/shorten", h.ShortenJSONHandler)
		r.Post("/api/shorten/batch", h.BatchShortenHandler)

		r.Get("/api/user/urls", h.GetUserURLs)
		r.Delete("/api/user/urls", h.DeleteUrls)
	})

	// Recorder для захвата охвата
	w := httptest.NewRecorder()

	return router, w, cfg, repo
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

		// Добавляем валидную куку
		addAuthCookie(req)

		// Выполняем запрос
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

		// Добавляем валидную куку
		addAuthCookie(req)

		// Выполнение запроса
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
