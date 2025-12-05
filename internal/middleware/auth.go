package middleware

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"net/http"
)

// Ключ для контекста,приватный тип, чтобы никто не перезаписал
type ctxKey string

const UserIDKey ctxKey = "userID"

func Auth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Получаем userID из cookie
			userID, err := util.GetUserIDFromCookie(r)

			if err != nil || userID == uuid.Nil {
				// Cookie нет или битая, создаем нового userID
				userID = uuid.New()
				util.SetSignedCookie(w, userID)
			}

			// UserID в контекст, теперь хендлер может его взять
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			// Продолжаем цепочку
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
