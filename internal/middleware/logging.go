package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

func Logging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			logger.Info("request received",
				zap.String("method", r.Method),
				zap.String("uri", r.RequestURI))

			next.ServeHTTP(w, r)

			duration := time.Since(start)

			logger.Info("response sent",
				zap.Duration("duration", duration),
			)
		})
	}
}
