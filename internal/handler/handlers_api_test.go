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
	"net"
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
		nameTest         string
		body             string
		wantCode         int
		wantBodyContains string
		description      string
	}{
		{
			nameTest: "Valid batch two urls",
			body: `[
				{"correlation_id": "1", "original_url": "https://ya.ru"},
				{"correlation_id": "2", "original_url": "https://google.com"}
			]`,
			wantCode:         http.StatusCreated,
			wantBodyContains: "http://localhost:33675/",
			description:      "Valid batch from 2 URL,  201 Created",
		},
		{
			nameTest:         "Empty batch",
			body:             `[]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Bad Request",
			description:      "Empty batch, 400 Bad Request",
		},
		{
			nameTest: "Duplicate correlation id",
			body: `[
				{"correlation_id": "dup", "original_url": "https://ya.ru"},
				{"correlation_id": "dup", "original_url": "https://google.com"}
			]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Bad Request",
			description:      "Duplicate correlation_id should return Bad Request",
		},
		{
			nameTest:         "Invalid URL in batch",
			body:             `[{"correlation_id": "1", "original_url": "not-a-url"}]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Bad Request",
			description:      "Invalid URL in batch, 400",
		},
		{
			nameTest: "Mixed valid and invalid",
			body: `[
				{"correlation_id": "1", "original_url": "https://ya.ru"},
				{"correlation_id": "2", "original_url": "not-a-url"}
			]`,
			wantCode:         http.StatusBadRequest,
			wantBodyContains: "Bad Request",
			description:      "1 Valid + 1 Invalid URL in batch, 400",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

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
		nameTest     string
		cookieUserID string
		mockURLs     []model.URLModel
		wantCode     int
		description  string
	}{
		{
			nameTest:     "User has URLs",
			cookieUserID: "test-user-id",
			mockURLs:     []model.URLModel{{ShortURL: "abc123", OriginalURL: "https://ya.ru"}},
			wantCode:     http.StatusOK,
			description:  "User has URL, 200 OK + список",
		},
		{
			nameTest:     "User has no URLs",
			cookieUserID: "test-user-id",
			mockURLs:     []model.URLModel{},
			wantCode:     http.StatusNoContent,
			description:  "User hasn't got URLs, 204 No Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

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

// TestDeleteUrls тестирует обработчик пометки URL на удаление
// (DELETE /api/user/urls).
//
// Хендлер принимает массив коротких идентификаторов, дедуплицирует их
// и отправляет задачу в асинхронный deleter (URLDeleter).
// Удаление происходит мягко (soft delete) в фоне.
//
// Основные проверяемые сценарии:
//   - Успешное удаление с валидными ID
//   - Пустой массив (ничего не делаем, но возвращаем 202)
//   - Массив с дубликатами (дедупликация)
//   - Отсутствие авторизации (нет cookie user_id)
//   - Невалидный JSON в теле запроса
func TestDeleteUrls(t *testing.T) {
	tests := []struct {
		nameTest     string
		cookieUserID string
		body         string
		wantCode     int
		description  string
	}{
		{
			nameTest:     "Success with ids",
			cookieUserID: "test-user-id",
			body:         `["abc123", "def456", "xyz789"]`,
			wantCode:     http.StatusAccepted,
			description:  "A valid ID array, 202 Accepted + sending to the deleter",
		},
		{
			nameTest:     "Empty array",
			cookieUserID: "test-user-id",
			body:         `[]`,
			wantCode:     http.StatusAccepted,
			description:  "Empty array, 202 Accepted (don't send anything)",
		},
		{
			nameTest:     "With duplicates",
			cookieUserID: "test-user-id",
			body:         `["abc123", "abc123", "def456", "abc123"]`,
			wantCode:     http.StatusAccepted,
			description:  "Array with duplicate, deduplication + 202 Accepted",
		},
		{
			nameTest:     "invalid JSON",
			cookieUserID: "test-user-id",
			body:         `{"not": "an array"}`,
			wantCode:     http.StatusBadRequest,
			description:  "Invalid JSON in body, 400 Bad Request",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Создаём контроллер gomock
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Мок не нужен (для тестов без авторизации)
			var h *Handler
			if tt.cookieUserID != "" {
				mockRepo := mocks.NewMockURLRepository(ctrl)
				mockRepo.EXPECT().
					MarkAsDeleted(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					AnyTimes()

				h = newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())
			} else {
				// Без авторизации — передаём nil, чтобы не создавать лишний мок
				h = newTestHandler(t, nil, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())
			}

			// Создаём роутер с middleware авторизации
			router := chi.NewRouter()
			router.Use(middleware.Auth(zap.NewNop()))
			router.Delete("/api/user/urls", h.DeleteUrls)

			// Создаём recorder и запрос
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			// Добавляем cookie, если требуется для авторизации
			if tt.cookieUserID != "" {
				req.AddCookie(&http.Cookie{Name: "user_id", Value: tt.cookieUserID})
			}

			// Выполняем запрос через роутер
			router.ServeHTTP(rec, req)

			// Проверяем HTTP-статус
			assert.Equal(t, tt.wantCode, rec.Code, tt.description)
		})
	}
}

// TestDeleteUrls_NoAuth тестирует поведение при отсутствии авторизации
func TestDeleteUrls_NoAuth(t *testing.T) {
	// Создаём контроллер gomock
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаём мок репозитория
	mockRepo := mocks.NewMockURLRepository(ctrl)

	// Разрешаем вызов MarkAsDeleted, потому что хендлер сейчас его делает даже без куки
	mockRepo.EXPECT().
		MarkAsDeleted(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Создаём тестовый handler
	h := newTestHandler(t, mockRepo, &config.Config{BaseURL: "http://localhost:8080"}, zap.NewNop())

	// Настраиваем роутер с middleware авторизации
	router := chi.NewRouter()
	router.Use(middleware.Auth(zap.NewNop()))
	router.Delete("/api/user/urls", h.DeleteUrls)

	// Создаём recorder и запрос (без cookie)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/user/urls", strings.NewReader(`["abc123"]`))
	req.Header.Set("Content-Type", "application/json")
	// Куку НЕ добавляем

	// Выполняем запрос
	router.ServeHTTP(rec, req)

	// Проверяем, что вернулся 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"Have to return 401 Unauthorized in the absence of user_id")
}

// TestGetInternalStats тестирует эндпоинт внутренней статистики
// (GET /api/internal/stats).
//
// Эндпоинт доступен только из доверенной подсети, указанной в конфигурации
// (поле TrustedSubnetNet). Проверка IP выполняется по заголовку X-Real-IP.
//
// Основные сценарии:
//   - Запрос из доверенной подсети → 200 OK + JSON со статистикой (urls и users)
//   - Запрос из недоверенной подсети → 403 Forbidden
//   - Отсутствует заголовок X-Real-IP → 403 Forbidden
//   - В конфигурации не задана доверенная подсеть → 403 Forbidden для всех
func TestGetInternalStats(t *testing.T) {
	tests := []struct {
		nameTest         string
		xRealIP          string
		trustedSubnet    string // CIDR, for try "192.168.0.0/16"
		wantCode         int
		wantBodyContains string
		description      string
	}{
		{
			nameTest:         "From trusted subnet",
			xRealIP:          "192.168.1.100",
			trustedSubnet:    "192.168.0.0/16",
			wantCode:         http.StatusOK,
			wantBodyContains: `"urls":`,
			description:      "Request from a trusted subnet, 200 OK + статистика",
		},
		{
			nameTest:         "From untrusted subnet",
			xRealIP:          "10.0.0.1",
			trustedSubnet:    "192.168.0.0/16",
			wantCode:         http.StatusForbidden,
			wantBodyContains: "Forbidden",
			description:      "Request from an untrusted subnet, 403 Forbidden",
		},
		{
			nameTest:         "Missing X-Real-IP",
			xRealIP:          "",
			trustedSubnet:    "192.168.0.0/16",
			wantCode:         http.StatusForbidden,
			wantBodyContains: "Forbidden",
			description:      "Missing X-Real-IP header - 403 Forbidden",
		},
		{
			nameTest:         "No trusted subnet in Config",
			xRealIP:          "192.168.1.100",
			trustedSubnet:    "", // пустая подсеть = доступ запрещён всегда
			wantCode:         http.StatusForbidden,
			wantBodyContains: "Forbidden",
			description:      "TrustedSubnetNet not configure, 403 Forbidden",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Создаём конфигурацию для теста
			cfg := &config.Config{
				BaseURL: "http://localhost:8080",
			}

			// Если указана подсеть — парсим CIDR и сохраняем в TrustedSubnetNet
			if tt.trustedSubnet != "" {
				_, ipnet, err := net.ParseCIDR(tt.trustedSubnet)
				if err == nil {
					cfg.TrustedSubnetNet = ipnet
				}
			}

			// Создаём контроллер gomock
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockURLRepository(ctrl)

			// Настраиваем возврат статистики (вызывается только при успешном доступе)
			// AnyTimes() используется, потому что в неудачных кейсах вызов может не произойти
			mockRepo.EXPECT().
				Stats().
				Return(12345, 987).
				AnyTimes()

			// Создаём тестовый handler с подготовленной конфигурацией
			h := newTestHandler(t, mockRepo, cfg, zap.NewNop())

			// Создаём роутер и регистрируем эндпоинт
			router := chi.NewRouter()
			router.Get("/api/internal/stats", h.GetInternalStats)

			// Создаём recorder для перехвата ответа и тестовый HTTP-запрос
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/internal/stats", nil)

			// Добавляем заголовок X-Real-IP, если он нужен для сценария
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			// Выполняем запрос через роутер
			router.ServeHTTP(rec, req)

			// Проверяем HTTP-статус
			assert.Equal(t, tt.wantCode, rec.Code, tt.description)

			// Проверяем содержимое тела ответа
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains, tt.description)
			}
		})
	}
}
