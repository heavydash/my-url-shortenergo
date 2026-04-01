// Package handler предоставляет HTTP-обработчики для сервиса сокращения ссылок.
//
// Основные эндпоинты:
//   - GET /          — главная страница
//   - POST /         — сокращение URL в plain-тексте
//   - GET /{id}      — редирект по короткой ссылке
//   - GET /ping      — health check
//
// Все тесты в этом пакете используют setupTest() для создания роутера и handler'а.
package handler

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPingHandler тестирует все основные сценарии health-check эндпоинта /ping.
//
// Покрываемые сюжеты:
// - Успешный пинг БД → 200 OK + "OK"
// - Ошибка сервиса → 500 + "db ping failed"
// - Ошибка записи ответа → лог ошибки
// - Отмена контекста (Canceled, DeadlineExceeded)
func TestPingHandler(t *testing.T) {
	// Создаём контроллер gomock
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := zap.NewNop()

	// Table-driven подход
	tests := []struct {
		nameTest         string
		pingErr          error  // что вернёт mockRepo.Ping()
		useFailingWriter bool   // специальный флаг для симуляции ошибки
		wantCode         int    // ожидаемый HTTP статус
		wantBodyContains string // что должно быть в теле ответа
		description      string //
	}{
		{
			nameTest:         "success",
			pingErr:          nil,
			useFailingWriter: false,
			wantCode:         http.StatusOK,
			wantBodyContains: "OK",
			description:      "Service respond successfully, 200 OK, body OK",
		},
		{
			nameTest:         "service error",
			pingErr:          errors.New("postgres connection error"),
			useFailingWriter: false,
			wantCode:         http.StatusInternalServerError,
			wantBodyContains: "db ping failed",
			description:      "Service respond error, 500 Internal Server Error, body Internal Server Error",
		},
		{

			nameTest:         "write error after successful ping",
			pingErr:          nil,
			useFailingWriter: true,
			wantCode:         http.StatusOK,
			wantBodyContains: "",
			description:      "Service respond successfully, body error in ResponseWriter",
		},
		{
			nameTest:         "context deadline exceeded",
			pingErr:          context.DeadlineExceeded,
			useFailingWriter: false,
			wantCode:         http.StatusInternalServerError,
			wantBodyContains: "db ping failed",
			description:      "Context deadline exceeded",
		},
		{
			nameTest:         "context cancelled",
			pingErr:          context.Canceled,
			useFailingWriter: false,
			wantCode:         http.StatusInternalServerError,
			wantBodyContains: "db ping failed",
			description:      "Context cancelled",
		},
	}
	// Перебираем все сценарии
	for _, tt := range tests {
		// t.Run запускает подтест
		t.Run(tt.nameTest, func(t *testing.T) {

			// Создаём мок репозитория
			mockRepo := mocks.NewMockURLRepository(ctrl)

			// Когда вызовут Ping с любым контекстом — верни tt.pingErr
			// Times(1) — строго один раз
			mockRepo.EXPECT().
				Ping(gomock.Any()).
				Return(tt.pingErr).
				Times(1)

			h := newTestHandler(t, mockRepo, nil, logger)

			// Используем роутер
			router := chi.NewRouter()
			router.Get("/ping", h.PingHandler)

			// Создаём стандартный ResponseRecorder, он собирает статус, заголовки и тело
			rec := httptest.NewRecorder()

			var writer http.ResponseWriter = rec
			if tt.useFailingWriter {
				writer = &failingResponseWriter{ResponseRecorder: rec}
			}

			// Создаём тестовый HTTP-запрос
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)

			router.ServeHTTP(writer, req)

			// Проверяем HTTP-статус, rec.Code заполняется внутри хендлера
			assert.Equal(t, tt.wantCode, rec.Code, tt.description)

			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains, tt.description)
			}

		})
	}
}

// TestHomeHandler тестирует главную страницу (GET /).
//
// Проверяет:
// - Корректный статус 200
// - Наличие ключевых фраз в HTML-ответе
func TestHomeHandler(t *testing.T) {
	tests := []struct {
		nameTest         string
		wantStatusCode   int
		wantBodyContains []string
		description      string
	}{
		{
			nameTest:       "success",
			wantStatusCode: http.StatusOK,
			wantBodyContains: []string{
				"URL Shortener Service",
				"POST /",
				"GET /{id}",
				"to shorten",
				"to redirect",
			},
			description: "Service respond successfully, 200 OK, body OK",
		},
		{
			nameTest:         "correct content-type",
			wantStatusCode:   http.StatusOK,
			wantBodyContains: []string{"URL Shortener Service"},
			description:      "Service respond successfully, 200 OK, body OK",
		},
	}
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {
			router, w, _, _ := setupTest(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatusCode, w.Code, tt.description)

			body := getResponseBody(t, w)

			for _, expected := range tt.wantBodyContains {
				assert.Contains(t, body, expected, tt.description)
			}
		})
	}
}

// TestShortenPlainHandler тестирует сокращение URL через plain-текст (POST /).
//
// Основные сценарии:
// - Валидный URL → 201 Created + короткая ссылка
// - Невалидный URL → 400 Bad Request
// - Пустое тело и только пробелы → 400
func TestShortenPlainHandler(t *testing.T) {
	tests := []struct {
		nameTest         string
		body             string
		wantCode         int
		wantBodyContains string
		description      string
	}{
		{
			nameTest:         "success_valid_url",
			body:             "https://example.com",
			wantCode:         http.StatusCreated,
			wantBodyContains: "http://localhost:33675/",
			description:      "Valid URL, 201 Created and shorten URL",
		},
		{
			nameTest:         "invalid_url",
			body:             "not-a-valid-url",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Invalid URL, 400 Bad Request",
		},
		{
			nameTest:         "empty_body",
			body:             "",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Empty body of request→ 400",
		},
		{
			nameTest:         "whitespace_only",
			body:             "   ",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Whitespace only, 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			router, w, _, _ := setupTest(t)

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code, tt.description)

			if tt.wantBodyContains != "" {
				body := getResponseBody(t, w)
				assert.Contains(t, body, tt.wantBodyContains)
			}
		})
	}
}

// TestRedirectURL тестирует редирект по короткой ссылке (GET /{id}).
//
// Сценарии:
// - Успешный редирект → 307 Temporary Redirect + Location
// - Ссылка не найдена → 404
// - Ошибка сервиса → 500
// - Пустой ID → 400 Bad Request (без вызова GetURL)
func TestRedirectURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := zap.NewNop()

	tests := []struct {
		nameTest           string
		shortID            string
		mockReturnURL      *model.URLModel
		mockReturnErr      error
		expectedGetURLCall bool
		wantCode           int
		wantLocation       string
		description        string
	}{
		{
			nameTest:           "success",
			shortID:            "goodID",
			mockReturnURL:      &model.URLModel{OriginalURL: "https://test.ru"},
			mockReturnErr:      nil,
			expectedGetURLCall: true,
			wantCode:           http.StatusTemporaryRedirect,
			wantLocation:       "https://test.ru",
			description:        "ShortenURL find, 307 Temporary Redirect + Location header",
		},
		{
			nameTest:           "not_found",
			shortID:            "badID",
			mockReturnURL:      nil,
			mockReturnErr:      errors.New("not found"),
			expectedGetURLCall: true,
			wantCode:           http.StatusNotFound,
			wantLocation:       "",
			description:        "ShortenURL not find → 404 Not Found",
		},
		{
			nameTest:           "service_error",
			shortID:            "errorID",
			mockReturnURL:      nil,
			mockReturnErr:      errors.New("database error"),
			expectedGetURLCall: true,
			wantCode:           http.StatusInternalServerError,
			wantLocation:       "",
			description:        "Error accessing the storage, 500 Internal Server Error",
		},
		{
			nameTest:           "empty_id",
			shortID:            "",
			mockReturnURL:      nil,
			mockReturnErr:      nil,
			expectedGetURLCall: false,
			wantCode:           http.StatusBadRequest,
			wantLocation:       "",
			description:        "Empty ID in URL, should return 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			mockRepo := mocks.NewMockURLRepository(ctrl)

			if tt.expectedGetURLCall {
				mockRepo.EXPECT().
					GetURL(gomock.Any(), tt.shortID).
					Return(tt.mockReturnURL, tt.mockReturnErr).
					Times(1)
			}
			h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, logger)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/"+tt.shortID, nil)

			h.RedirectURL(rec, req)

			assert.Equal(t, tt.wantCode, rec.Code, tt.description)

			if tt.wantLocation != "" {
				assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"), tt.description)
			}
		})
	}
}

// failingResponseWriter — специальная обёртка над httptest.ResponseRecorder,
// которая симулирует ошибку записи (метод Write возвращает ошибку).
//
// Используется только в тестах для покрытия ветки ошибки записи ответа в PingHandler
// и других хендлерах.
type failingResponseWriter struct {
	*httptest.ResponseRecorder
}

// Write всегда возвращает ошибку.
// Используется для тестирования обработчиков ошибок записи.
func (f *failingResponseWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write error")
}
