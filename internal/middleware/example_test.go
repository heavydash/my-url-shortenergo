// middleware/example_test.go
package middleware

import (
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Пример работы GzipMiddleware со сжатием ответа.
func ExampleGzipMiddleware() {
	logger, _ := zap.NewDevelopment()

	// Создаем обработчик, возвращающий текст для сжатия
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("This response will be compressed by gzip middleware"))
	})

	// Оборачиваем в gzip middleware
	gzippedHandler := GzipMiddleware(logger)(handler)

	// Запрос с поддержкой gzip
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	gzippedHandler.ServeHTTP(w, req)

	fmt.Printf("Status: %d\n", w.Code)
	fmt.Printf("Content-Encoding: %s\n", w.Header().Get("Content-Encoding"))
	fmt.Printf("Body is gzipped: %v\n", w.Header().Get("Content-Encoding") == "gzip")

}

// Пример логирования запросов.
func ExampleLogging() {
	// Создаем логгер с выводом в буфер
	config := zap.NewDevelopmentConfig()
	config.OutputPaths = []string{"stdout"}
	logger, _ := config.Build()

	// Создаем простой обработчик
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	// Оборачиваем в logging middleware
	loggedHandler := Logging(logger)(handler)

	// Выполняем запрос
	req := httptest.NewRequest("POST", "/api/shorten", strings.NewReader("https://example.com"))
	w := httptest.NewRecorder()

	loggedHandler.ServeHTTP(w, req)

	fmt.Printf("Request logged with method: %s\n", req.Method)
	fmt.Printf("Request logged with path: %s\n", req.URL.Path)

}

// Пример цепочки middleware.
func Example_middlewareChain() {
	logger, _ := zap.NewDevelopment()

	// Создаем цепочку middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(UserIDKey)
		if userID != nil {
			_, _ = fmt.Fprint(w, "Authenticated request")
		} else {
			_, _ = fmt.Fprint(w, "Anonymous request")
		}
	})

	// Порядок middleware важен:
	// 1. Logging (логирует полное время выполнения)
	// 2. Gzip (сжимает ответ)
	// 3. Auth (добавляет userID в контекст)
	chain := Logging(logger)(
		GzipMiddleware(logger)(
			Auth(logger)(
				handler,
			),
		),
	)

	// Тестовый запрос
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	chain.ServeHTTP(w, req)

	fmt.Printf("Chain executed successfully\n")
	fmt.Printf("Has auth cookie: %v\n", w.Header().Get("Set-Cookie") != "")

}
