// Package middleware предоставляет middleware-компоненты для HTTP-сервера.
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter оборачивает http.ResponseWriter для прозрачного сжатия данных.
// Реализует интерфейсы http.ResponseWriter, http.Flusher для корректной работы
// с сетевыми соединениями и стриминговыми ответами.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

// Write записывает сжатые данные в исходящий поток.
// Реализует интерфейс io.Writer, автоматически сжимая передаваемые данные.
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// Flush принудительно отправляет буферизированные сжатые данные клиенту.
// Выполняет двойной flush: сначала для gzip-буфера, затем для сетевого соединения.
// Это важно для streaming-ответов и серверных событий (Server-Sent Events).
func (w *gzipResponseWriter) Flush() {
	// Flush gzip буффера
	if f, ok := w.Writer.(flusher); ok {
		f.Flush()
	}
	// Flush оригинального соединения
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// flusher определяет интерфейс для объектов, поддерживающих принудительную отправку буфера.
// В основном используется gzip.Writer для отправки сжатых данных до закрытия потока.
type flusher interface {
	Flush() error
}

// gzipWriterPool - пул объектов gzip.Writer для уменьшения аллокаций памяти.
// Пул используется для повторного использования уже созданных gzip.Writer'ов,
// что значительно улучшает производительность при частых сжатиях.
var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		// Шаблонный Writer с nil
		return gzip.NewWriter(nil)
	},
}

// GzipMiddleware предоставляет middleware для прозрачного сжатия HTTP-трафика.
//
// Middleware выполняет двустороннее сжатие:
//   - Распаковка входящих запросов с заголовком Content-Encoding: gzip
//   - Сжатие исходящих ответов если клиент поддерживает gzip (Accept-Encoding: gzip)
//
// Поддерживает:
//   - Пул gzip.Writer для уменьшения аллокаций
//   - Flush для streaming-ответов
//   - Автоматическое определение необходимости сжатия
//
// Параметры:
//   - next: следующий обработчик в цепочке middleware
//
// Возвращает:
//   - http.Handler: middleware-обработчик
//
// Пример использования:
//
//	router := chi.NewRouter()
//	router.Use(middleware.GzipMiddleware)
//	router.Get("/api/data", dataHandler)
//
// Пример заголовков запроса:
//
//	// Клиент отправляет сжатые данные:
//	POST /api/data
//	Content-Encoding: gzip
//	Body: [gzip compressed data]
//
//	// Клиент запрашивает сжатые данные:
//	GET /api/data
//	Accept-Encoding: gzip
//
// Примечания:
//   - Сжатие применяется только к текстовым типам контента (по умолчанию в Go)
//   - Для больших файлов рекомендуется использовать отдельные эндпоинты без сжатия
//   - Минимальный размер ответа для сжатия обычно 1KB (зависит от реализации)
//   - Добавляет заголовок "Vary: Accept-Encoding" для корректного кэширования
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			r.Body = gz
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Устанавливаем заголовки
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")

		// Берем Writer из пула
		gz := gzipWriterPool.Get().(*gzip.Writer)

		// Очищение состояния предыдущего использования
		// Не аллоцируя новые структуры внутри
		gz.Reset(w)

		// Оборачиваем в структуру
		gw := &gzipResponseWriter{
			Writer:         gz,
			ResponseWriter: w,
		}

		// Передаем управление следующему хендлеру
		next.ServeHTTP(gw, r)

		// Закрываем и возвращаем в пул
		gz.Close()

		// Возвращаем объект обратно в пул
		gzipWriterPool.Put(gz)
	})
}
