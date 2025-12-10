package middleware

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"go.uber.org/zap"
	"net/http"
)

// Ключ для контекста,приватный тип, чтобы никто не перезаписал
type ctxKey string

const UserIDKey ctxKey = "userID"

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
				} else {
					userID = parsedID
					logger.Info("Auth: parsed userID", zap.String("user_id", userID.String()))
				}
			}

			// Ставим куку, чтобы тест её увидел на следующем запросе
			util.SetSignedCookie(w, userID)

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
