// Package middleware предоставляет middleware-компоненты для HTTP-сервера.
package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Logging создает middleware для логирования HTTP-запросов и ответов.
//
// Middleware логирует:
//   - Входящие запросы (метод, URI)
//   - Исходящие ответы (время обработки)
//   - Время выполнения запроса
//
// Логи записываются в формате структурированного лога с использованием zap.Logger.
// Это позволяет легко агрегировать и анализировать логи в системах мониторинга
// (ELK Stack, Grafana Loki, Datadog и др.).
//
// Параметры:
//   - logger: структурированный логгер zap.Logger для записи событий
//
// Возвращает:
//   - func(http.Handler) http.Handler: middleware функцию
//
// Пример использования:
//
//	router := chi.NewRouter()
//	router.Use(middleware.Logging(logger))
//	router.Get("/api/shorten", handler.ShortenURL)
//
// Пример вывода логов:
//
//	{
//	  "level": "info",
//	  "ts": "2024-01-15T10:30:00.123Z",
//	  "msg": "request received",
//	  "method": "POST",
//	  "uri": "/api/shorten"
//	}
//	{
//	  "level": "info",
//	  "ts": "2024-01-15T10:30:00.125Z",
//	  "msg": "response sent",
//	  "duration": "2.345ms"
//	}
//
// Примечания:
//   - Логирование происходит до и после выполнения основного обработчика
//   - Время измеряется с наносекундной точностью
//   - Для production рекомендуется использовать уровень логирования INFO
//   - Включайте это middleware ПЕРВЫМ в цепочке, чтобы замерять полное время
//   - Для дебагга можно добавить дополнительные поля (user-agent, IP и т.д.)
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
