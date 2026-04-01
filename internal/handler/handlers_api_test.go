// Package handler содержит тесты HTTP-обработчиков API сервиса сокращения URL.
//
// В этом файле тестируются JSON и batch эндпоинты, а также получение списка
// ссылок пользователя. Тесты используют table-driven подход, gomock-моки репозитория,
// chi-роутер и middleware.Auth для максимальной приближенности к реальной работе.
//
// Основные тестируемые эндпоинты:
//   - POST /api/shorten          — сокращение одиночной ссылки через JSON
//   - POST /api/shorten/batch    — пакетное сокращение ссылок
//   - GET /api/user/urls         — получение списка ссылок текущего пользователя
//
// Все тесты используют вспомогательные функции setupTest() и newTestHandler().
package handler

import (
	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestShortenJSONHandler тестирует обработчик сокращения одиночной ссылки
// через JSON (POST /api/shorten).
//
// Тест использует таблицу сценариев и проверяет корректную обработку различных
// входных данных: валидный JSON, невалидный URL, некорректный синтаксис JSON,
// пустое тело и неправильный Content-Type.
//
// Основные проверяемые аспекты:
//   - Возврат правильного HTTP-статуса (201 Created или 400 Bad Request).
//   - Формирование полного короткого URL в ответе.
//   - Корректная обработка ошибок валидации.
func TestShortenJSONHandler(t *testing.T) {
	tests := []struct {
		nameTest         string
		body             string
		contentType      string
		wantCode         int
		wantBodyContains string
		description      string
	}{
		{
			nameTest:         "Valid JSON",
			body:             `{"url":"https://example.com"}`,
			contentType:      "application/json",
			wantCode:         http.StatusCreated,
			wantBodyContains: "http://localhost:33675/",
			description:      "Valid JSON with correct URL",
		},
		{
			nameTest:         "Invalid JSON",
			body:             `{"url":"not-a-valid-url"}`,
			contentType:      "application/json",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Invalid URL in JSON",
		},
		{
			nameTest:         "Invalid JSON syntax",
			body:             `{"url": "https://example.com"`, // JSON не закрыт
			contentType:      "application/json",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Invalid JSON syntax",
		},
		{
			nameTest:         "Empty body",
			body:             ``,
			contentType:      "application/json",
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Invalid request",
			description:      "Empty bode in JSON-request",
		},
		{
			nameTest:         "Wrong content type",
			body:             `{"url": "https://example.com"}`,
			contentType:      "text/plain", // неправильный тип
			wantCode:         http.StatusCreated,
			wantBodyContains: "http://localhost:33675/",
			description:      "Invalid Content-Type",
		},
	}
	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Создаём роутер, recorder и тестовый handler через вспомогательную функцию
			router, w, _, _ := setupTest(t)

			// Создаём HTTP-запрос с телом и заголовком Content-Type
			req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			// Выполняем запрос через роутер
			router.ServeHTTP(w, req)

			// Проверяем HTTP-статус
			assert.Equal(t, tt.wantCode, w.Code, tt.description)

			// Проверяем содержимое тела ответа
			if tt.wantBodyContains != "" {
				body := getResponseBody(t, w)
				assert.Contains(t, body, tt.wantBodyContains, tt.description)
			}
		})
	}
}

// TestBatchShortenHandler тестирует обработчик пакетного сокращения ссылок
// (POST /api/shorten/batch).
//
// Тест проверяет корректную работу с массивом URL, включая валидацию,
// обработку дубликатов correlation_id и смешанные сценарии (валидные + невалидные URL).
func TestBatchShortenHandler(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		wantCode         int
		wantBodyContains string
		description      string
	}{
		{
			name: "Valid batch two urls",
			body: `[
				{"correlation_id": "1", "original_url": "https://ya.ru"},
				{"correlation_id": "2", "original_url": "https://google.com"}
			]`,
			wantCode:         http.StatusCreated,
			wantBodyContains: "http://localhost:33675/",
			description:      "Valid batch from 2 URL,  201 Created",
		},
		{
			name:             "Empty batch",
			body:             `[]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "empty batch",
			description:      "Empty batch, 400 Bad Request",
		},
		{
			name: "Duplicate correlation id",
			body: `[
				{"correlation_id": "dup", "original_url": "https://ya.ru"},
				{"correlation_id": "dup", "original_url": "https://google.com"}
			]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "duplicate correlation_id",
			description:      "Duplicate correlation_id,  400",
		},
		{
			name:             "Invalid URL in batch",
			body:             `[{"correlation_id": "1", "original_url": "not-a-url"}]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "invalid url",
			description:      "Invalid URL in batch, 400",
		},
		{
			name: "Mixed valid and invalid",
			body: `[
				{"correlation_id": "1", "original_url": "https://ya.ru"},
				{"correlation_id": "2", "original_url": "not-a-url"}
			]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "invalid url",
			description:      "1 Valid + 1 Invalid URL in batch, 400",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Создаём роутер и recorder
			router, w, _, _ := setupTest(t)

			// Создаём HTTP-запрос с JSON-телом
			req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			// Выполняем запрос
			router.ServeHTTP(w, req)

			// Проверяем статус ответа
			assert.Equal(t, tt.wantCode, w.Code, tt.description)

			// Проверяем содержимое тела ответа
			if tt.wantBodyContains != "" {
				body := getResponseBody(t, w)
				assert.Contains(t, body, tt.wantBodyContains, tt.description)
			}
		})
	}
}

// TestGetUserURLs тестирует получение списка коротких ссылок текущего пользователя
// (GET /api/user/urls).
//
// Тест проверяет два основных сценария:
//   - У пользователя есть ссылки - возвращается 200 OK и список.
//   - У пользователя нет ссылок - возвращается 204 No Content.
//
// Используется мок-репозиторий для контроля возвращаемых данных.
func TestGetUserURLs(t *testing.T) {
	tests := []struct {
		name         string
		cookieUserID string
		mockURLs     []model.URLModel
		wantCode     int
		description  string
	}{
		{
			name:         "User has URLs",
			cookieUserID: "test-user-id",
			mockURLs:     []model.URLModel{{ShortURL: "abc123", OriginalURL: "https://ya.ru"}},
			wantCode:     http.StatusOK,
			description:  "User has URL, 200 OK + список",
		},
		{
			name:         "User has no URLs",
			cookieUserID: "test-user-id",
			mockURLs:     []model.URLModel{},
			wantCode:     http.StatusNoContent,
			description:  "User hasn't got URLs, 204 No Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Создаём контроллер gomock
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Создаём мок репозитория
			mockRepo := mocks.NewMockURLRepository(ctrl)

			// Настраиваем ожидание вызова GetURLsByUser
			if tt.cookieUserID != "" {
				mockRepo.EXPECT().
					GetURLsByUser(gomock.Any(), gomock.Any()).
					Return(tt.mockURLs, nil).
					Times(1)
			}

			// Создаём тестовый handler
			h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())

			// Настраиваем роутер с middleware авторизации
			router := chi.NewRouter()
			router.Use(middleware.Auth(zap.NewNop())) // добавляем auth middleware
			router.Get("/api/user/urls", h.GetUserURLs)

			// Создаём recorder и запрос
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)

			// Добавляем cookie, если требуется
			if tt.cookieUserID != "" {
				req.AddCookie(&http.Cookie{Name: "user_id", Value: tt.cookieUserID})
			}

			// Выполняем запрос
			router.ServeHTTP(rec, req)

			// Проверяем статус
			assert.Equal(t, tt.wantCode, rec.Code, tt.description)
		})
	}
}

// TestGetUserURLs_NoAuth тестирует поведение обработчика GetUserURLs при отсутствии
// авторизации (нет cookie "user_id").
//
// Ожидаемое поведение: возвращается статус 401 Unauthorized.
// Тест специально не добавляет куку, чтобы проверить работу middleware.Auth.
func TestGetUserURLs_NoAuth(t *testing.T) {
	// Создаём контроллер gomock
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаём мок репозитория
	mockRepo := mocks.NewMockURLRepository(ctrl)

	// Метод GetURLsByUser может быть вызван (с uuid.Nil), но результат не важен
	mockRepo.EXPECT().
		GetURLsByUser(gomock.Any(), gomock.Any()).
		Return([]model.URLModel{}, nil).
		AnyTimes()

	// Создаём тестовый handler
	h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())

	// Настраиваем роутер с middleware авторизации
	router := chi.NewRouter()
	router.Use(middleware.Auth(zap.NewNop()))
	router.Get("/api/user/urls", h.GetUserURLs)

	// Создаём recorder и запрос (без cookie)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/urls", nil)
	// Куку НЕ добавляем специально!

	// Выполняем запрос
	router.ServeHTTP(rec, req)

	// Проверяем, что вернулся 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "Have to return 401 Unauthorized in the absence of user_id")
}
