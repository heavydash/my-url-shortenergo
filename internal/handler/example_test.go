// internal/handler/example_test.go
package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"go.uber.org/zap"
)

// Пример сокращения URL через текстовый эндпоинт.
func ExampleHandler_ShortenPlainHandler() {
	// Инициализация зависимостей
	logger, _ := zap.NewDevelopment()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	repo := repository.NewMemoryRepository("http://localhost:8080")

	// Конфигурация
	cfg := &config.Config{
		DeletionQueueBuffer:   1000,
		DeletionFlushInterval: 500 * time.Millisecond,
		DeletionMaxBatchSize:  1000,
		BaseURL:               "http://localhost:8080",
	}

	// Сервис аудита (Noop, т.к. AuditFilePath и AuditRemoteURL не заданы)
	auditSvc := service.New(cfg, logger)
	defer func() {
		if err := auditSvc.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()

	// Создаем handler через конструктор
	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// Создание тестового запроса
	reqBody := "https://example.com/very/long/url/path"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()

	// Вызов обработчика
	h.ShortenPlainHandler(w, req)

	// Проверка результата
	fmt.Printf("Status: %d\n", w.Code)
	if w.Code == http.StatusCreated {
		fmt.Printf("Response contains shortened URL: %v\n",
			strings.HasPrefix(strings.TrimSpace(w.Body.String()), "http://"))
	}

	// Output pattern:
	// Status: 201
	// Response contains shortened URL: true
}

// Пример перенаправления по короткой ссылке.
func ExampleHandler_RedirectURL() {
	logger, _ := zap.NewDevelopment()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	repo := repository.NewMemoryRepository("http://localhost:8080")

	// Конфигурация
	cfg := &config.Config{
		DeletionQueueBuffer:   1000,
		DeletionFlushInterval: 500 * time.Millisecond,
		DeletionMaxBatchSize:  1000,
		BaseURL:               "http://localhost:8080",
	}

	// Сервис аудита
	auditSvc := service.New(cfg, logger)
	defer func() {
		if err := auditSvc.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()

	// Создаем handler
	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// Тестируем редирект на несуществующий URL
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	h.RedirectURL(w, req)

	fmt.Printf("Status for non-existent URL: %d\n", w.Code)

	// Output:
	// Status for non-existent URL: 404
}

// Пример проверки доступности БД через Ping.
func ExampleHandler_PingHandler() {
	logger, _ := zap.NewDevelopment()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	repo := repository.NewMemoryRepository("http://localhost:8080")

	// Конфигурация (может быть nil для тестов)
	var cfg *config.Config // nil config допустимо

	// Сервис аудита
	auditSvc := service.New(&config.Config{}, logger)
	defer func() {
		if err := auditSvc.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()

	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// Запрос ping
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	h.PingHandler(w, req)

	fmt.Printf("Ping Status: %d\n", w.Code)
	fmt.Printf("Ping Response: %s\n", strings.TrimSpace(w.Body.String()))

	// Output для MemoryRepository:
	// Ping Status: 200
	// Ping Response: OK
}

// Пример домашней страницы.
func ExampleHandler_HomeHandler() {
	logger, _ := zap.NewDevelopment()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	repo := repository.NewMemoryRepository("http://localhost:8080")

	// Конфигурация
	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}

	// Сервис аудита
	auditSvc := service.New(cfg, logger)
	defer func() {
		if err := auditSvc.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()

	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// Запрос на корневой путь
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.HomeHandler(w, req)

	fmt.Printf("Status: %d\n", w.Code)
	fmt.Printf("Response contains instructions: %v\n",
		strings.Contains(w.Body.String(), "POST") &&
			strings.Contains(w.Body.String(), "GET"))

	// Output:
	// Status: 200
	// Response contains instructions: true
}

// Пример с неправильным методом для HomeHandler.
func ExampleHandler_HomeHandler_wrongMethod() {
	logger, _ := zap.NewDevelopment()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	repo := repository.NewMemoryRepository("http://localhost:8080")
	cfg := &config.Config{}
	auditSvc := service.New(cfg, logger)
	defer func() {
		if err := auditSvc.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()

	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// POST запрос на корневой путь (должен вернуть 405)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()

	h.HomeHandler(w, req)

	fmt.Printf("Status for POST /: %d\n", w.Code)
	fmt.Printf("Error message: %s\n", strings.TrimSpace(w.Body.String()))

	// Output:
	// Status for POST /: 405
	// Error message: Method not allowed
}
