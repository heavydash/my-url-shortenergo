package middleware

import (
	"go.uber.org/zap"
	"net/http"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	status      int
	size        int
	wroteHeader bool
}

func (rw *responseRecorder) WriteHeader(code int) {
	rw.status = code
	rw.wroteHeader = true
}

func (rw *responseRecorder) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func Logging(logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseRecorder{
				ResponseWriter: w,
				status:         http.StatusOK,
				wroteHeader:    false,
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			logger.Info("request received",
				zap.String("method", r.Method),
				zap.String("url", r.URL.String()),
			)

			logger.Info("response sent",
				zap.Int("status", rw.status),
				zap.Int("size", rw.size),
				zap.Duration("duration", duration),
				zap.String("content_type", rw.Header().Get("Content-Type")),
			)
		})
	}
}
