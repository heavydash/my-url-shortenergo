// Пакет middleware предоставляет компоненты промежуточной обработки HTTP-запросов
// для сервиса сокращения URL.
//
// Middleware выполняют кросс-режущие задачи (cross-cutting concerns): аутентификацию,
// логирование, восстановление после паник и т.д. перед передачей управления
// основным обработчикам. Все middleware совместимы с chi.Router.Use().
//
// # Основные возможности
//
//   - Auth — stateless-аутентификация пользователей через signed cookie "user_id".
//   - ParseAuthHeader — парсинг и проверка токена авторизации (используется как в HTTP, так и в gRPC).
//   - UserIDKey — типобезопасный ключ для хранения идентификатора пользователя в контексте.
//
// # Пример использования
//
//	router := chi.NewRouter()
//	router.Use(middleware.Auth(logger))
//	router.Post("/api/shorten", handler.ShortenURL)
//
// В обработчике можно извлечь userID так:
//
//	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

package middleware

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"go.uber.org/zap"
	"net/http"
)

// ctxKey — приватный тип ключа для хранения значений в контексте.
//
// Используется для создания уникального ключа UserIDKey. Приватный тип
// гарантирует, что никто извне пакета не сможет случайно использовать тот же ключ
// и вызвать коллизию значений в контексте.
type ctxKey string

// UserIDKey — ключ для хранения идентификатора пользователя в контексте запроса.
//
// Значение под этим ключом имеет тип uuid.UUID. Используется после успешной
// аутентификации в middleware.Auth и в gRPC AuthInterceptor.
const userIDKey ctxKey = "userID"

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
//	    userID := r.Context().Value(middleware.userIDKey).(uuid.UUID)
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
					logger.Info("Auth: parsed userID",
						zap.String("user_id", userID.String()),
						zap.String("cookie_value", cookie.Value))
				}
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID извлекает userID из контекста.
// Возвращает uuid.Nil, если userID отсутствует или имеет неверный тип.
// Это предпочтительный способ получения userID как в HTTP-хендлерах, так и в gRPC.
func GetUserID(ctx context.Context) uuid.UUID {
	if ctx == nil {
		return uuid.Nil
	}

	val := ctx.Value(userIDKey)
	if val == nil {
		return uuid.Nil
	}

	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return userID
}

// ParseAuthHeader извлекает и проверяет userID из строки авторизации.
//
// Функция поддерживает два формата:
//   - "Bearer <signed-token>"
//   - просто "<signed-token>"
//
// Она удаляет префикс "Bearer ", проверяет подпись токена и возвращает uuid.UUID.
// В случае любой ошибки возвращает uuid.Nil и ошибку.
//
// Используется:
//   - в HTTP-обработчиках (если потребуется прямой парсинг заголовка),
//   - в gRPC AuthInterceptor для проверки metadata "authorization".
//
// Логика проверки полностью делегируется util.GetUserIDFromToken.
func ParseAuthHeader(authHeader string) (uuid.UUID, error) {
	if authHeader == "" {
		return uuid.Nil, errors.New("empty authorization header")
	}

	return util.GetUserIDFromToken(authHeader)
}

// SetUserIDToContext помещает userID в контекст под правильным (неэкспортируемым) ключом.
// Используется из других пакетов (например, gRPC), где нельзя напрямую обращаться к userIDKey.
func SetUserIDToContext(ctx context.Context, userID uuid.UUID) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userIDKey, userID)
}
