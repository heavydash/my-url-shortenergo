// Package middleware предоставляет middleware-компоненты для HTTP-сервера.
// Middleware выполняют обработку запросов перед их передачей основным обработчикам.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"go.uber.org/zap"
)

// Ключ для контекста, приватный тип чтобы гарантировать уникальность
// и предотвратить случайные коллизии с ключами из других пакетов.
type ctxKey string

// UserIDKey - ключ для хранения идентификатора пользователя в контексте запроса.
// Используется для извлечения userID в обработчиках после аутентификации.
const UserIDKey ctxKey = "userID"

// Auth создает middleware для аутентификации пользователей на основе cookies.
//
// Middleware выполняет следующие действия:
//  1. Проверяет наличие валидной куки "user_id" в запросе
//  2. Если кука отсутствует или невалидна - генерирует новый UUID и устанавливает куку
//  3. Если кука валидна - извлекает из нее UUID пользователя
//  4. Добавляет userID в контекст запроса для использования в обработчиках
//
// Параметры:
//   - logger: логгер для записи событий аутентификации
//
// Возвращает:
//   - func(http.Handler) http.Handler: middleware функцию
//
// Пример использования:
//
//	router := chi.NewRouter()
//	router.Use(middleware.Auth(logger))
//	router.Post("/api/shorten", handler.ShortenURL)
//
// Пример извлечения userID в обработчике:
//
//	func SomeHandler(w http.ResponseWriter, r *http.Request) {
//	    userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
//	    // ... использование userID
//	}
//
// Примечания:
//   - Куки подписываются для защиты от подделки
//   - Срок жизни куки устанавливается в утилите util.SetSignedCookie
//   - При невалидной куке генерируется новый пользователь (stateless подход)
//   - UUID сохраняется в контексте с приватным типом ключа для безопасности
func Auth(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var userID uuid.UUID

			cookie, err := r.Cookie("user_id")
			if err != nil || cookie == nil {
				// Нет куки, создаём нового пользователя
				logger.Info("Auth: no cookie", zap.Error(err))
				userID = uuid.New()
				util.SetSignedCookie(w, userID)
			} else {
				logger.Info("Auth: cookie found", zap.String("cookie_value", cookie.Value))
				// Есть кука, пытаемся распарсить
				parsedID, parseErr := util.GetUserIDFromCookie(r)
				if parseErr != nil {
					logger.Warn("Auth: parse cookie failed", zap.Error(parseErr))
					// Битая кука, создаём нового
					userID = uuid.New()
					util.SetSignedCookie(w, userID)
				} else {
					userID = parsedID
					logger.Info("Auth: parsed userID", zap.String("user_id", userID.String()))
					//нет SetSignedCookie - уже валидная
				}
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
