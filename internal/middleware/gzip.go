package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

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

type flusher interface {
	Flush() error
}

// Пул для gzip.Writer
var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		// Шаблонный Writer с nil
		return gzip.NewWriter(nil)
	},
}

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
