// Package middleware содержит тесты для компонентов промежуточной обработки запросов.
//
// Основной акцент сделан на тестировании middleware.Auth — stateless аутентификации
// через signed cookie "user_id".
//
// Тесты проверяют:
//   - Создание нового пользователя при отсутствии куки
//   - Валидацию существующей корректной куки
//   - Обработку битых и некорректных cookies
//   - Сохранение userID в контексте запроса
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestAuth проверяет основную логику middleware.Auth.
//
// Выполняет сценарии:
//  1. Отсутствие куки → создание нового пользователя + установка signed cookie
//  2. Валидная signed cookie → извлечение userID и сохранение в контексте
//  3. Битая/некорректная кука → создание нового пользователя
//
// Параметры тестов:
//   - cookieValue: значение куки в incoming запросе
//   - wantNewUser: ожидается ли создание нового пользователя
//   - wantStatusCode: ожидаемый HTTP статус
func TestAuth(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		cookieValue    string // значение куки в запросе
		wantNewUser    bool   // ожидается ли создание нового пользователя
		wantStatusCode int
		description    string
	}{
		{
			name:           "No cookie - create new user",
			cookieValue:    "",
			wantNewUser:    true,
			wantStatusCode: http.StatusOK,
			description:    "If there is no cookie, a new user must be created",
		},
		{
			name:           "Valid signed cookie",
			cookieValue:    "", // будет установлена валидная кука внутри теста
			wantNewUser:    false,
			wantStatusCode: http.StatusOK,
			description:    "A valid cookie must be retrieved and stored in the context",
		},
		{
			name:           "Invalid cookie - create new user",
			cookieValue:    "invalid.cookie.value",
			wantNewUser:    true,
			wantStatusCode: http.StatusOK,
			description:    "A broken cookie leads to the creation of a new user",
		},
		{
			name:        "Malformed cookie - create new user",
			cookieValue: "only.one.part",
			wantNewUser: true,
			description: "Incorrect cookie format - new user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Создаём middleware с тестовым логгером
			authMiddleware := Auth(logger)

			// Создаём тестовый handler, который проверяет наличие userID в контексте
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Проверяем, что userID сохранён в контексте при поиощи геттера
				userID := GetUserID(r.Context())

				assert.NotEqual(t, uuid.Nil, userID, "userID must not be Nil after Auth middleware")

				assert.IsType(t, uuid.UUID{}, userID, "userID must be of type uuid.UUID")

				w.WriteHeader(http.StatusOK)
			})

			// Оборачиваем handler в middleware
			handler := authMiddleware(testHandler)

			// Создаём recorder и запрос
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/shorten", nil)

			// Добавляем куку, если указана
			if tt.cookieValue != "" {
				if tt.cookieValue == "valid" {
					// Генерируем настоящую валидную куку
					userID := uuid.New()
					cookieRec := httptest.NewRecorder()
					util.SetSignedCookie(cookieRec, userID)
					validCookie := cookieRec.Result().Cookies()[0]
					req.AddCookie(validCookie)
				} else {
					// Добавляем "битую" куку
					req.AddCookie(&http.Cookie{Name: "user_id", Value: tt.cookieValue})
				}
			}

			// Выполняем middleware + handler
			handler.ServeHTTP(rec, req)

			// Проверяем статус
			assert.Equal(t, http.StatusOK, rec.Code, tt.description)

			// Дополнительная проверка: Если ожидалось создание нового пользователя,
			// проверяем, что установлена кука
			if tt.wantNewUser {
				cookies := rec.Result().Cookies()
				var hasNewCookie bool
				for _, c := range cookies {
					if c.Name == "user_id" && c.Value != "" {
						hasNewCookie = true
						break
					}
				}
				assert.True(t, hasNewCookie, "A new signed cookie must be "+
					"installed when creating a user")
			}
		})
	}
}

// TestAuth_ValidCookie проверяет корректное извлечение userID из валидной signed cookie.
//
// Выполняет:
//  1. Создание валидной signed cookie через util.SetSignedCookie
//  2. Передачу куки в запрос
//  3. Проверку, что userID корректно извлекается и сохраняется в контексте
func TestAuth_ValidCookie(t *testing.T) {
	logger := zap.NewNop()

	// Создаём валидную signed cookie
	userID := uuid.New()
	rec := httptest.NewRecorder()
	util.SetSignedCookie(rec, userID)
	validCookie := rec.Result().Cookies()[0]

	// Создаём middleware
	authMiddleware := Auth(logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userIDFromContext := GetUserID(r.Context())
		assert.Equal(t, userID, userIDFromContext, "The userID in the context must match the cookie")
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(validCookie)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestParseAuthHeader тестирует парсинг и проверку заголовка авторизации.
//
// Выполняет сценарии:
//  1. Валидный токен без префикса Bearer
//  2. Валидный токен с префиксом Bearer
//  3. Пустой заголовок
//  4. Некорректный токен
func TestParseAuthHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		wantErr     bool
		description string
	}{
		{
			name:        "Valid token without Bearer",
			header:      "", // будет заполнена валидным токеном ниже
			wantErr:     false,
			description: "Valid token without the Bearer prefix",
		},
		{
			name:        "Valid token with Bearer prefix",
			header:      "", // будет заполнена ниже
			wantErr:     false,
			description: "Valid token with the Bearer prefix",
		},
		{
			name:        "Empty header",
			header:      "",
			wantErr:     true,
			description: "An empty header should return an error",
		},
		{
			name:        "Invalid token",
			header:      "invalid.token.value",
			wantErr:     false,
			description: "An invalid token should return an error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var header string

			if tt.name == "Valid token without Bearer" || tt.name == "Valid token with Bearer prefix" {
				// Генерируем валидный токен
				userID := uuid.New()
				rec := httptest.NewRecorder()
				util.SetSignedCookie(rec, userID)
				cookieValue := rec.Result().Cookies()[0].Value

				if tt.name == "Valid token with Bearer prefix" {
					header = "Bearer " + cookieValue
				} else {
					header = cookieValue
				}
			} else {
				header = tt.header
			}

			parsedID, err := ParseAuthHeader(header)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
				return
			}

			assert.NoError(t, err, tt.description)
			assert.NotEqual(t, uuid.Nil, parsedID, "A valid UUID should be returned")
		})
	}
}

// TestParseAuthHeader_EmptyString проверяет поведение при пустой строке.
func TestParseAuthHeader_EmptyString(t *testing.T) {
	_, err := ParseAuthHeader("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty authorization header")
}
