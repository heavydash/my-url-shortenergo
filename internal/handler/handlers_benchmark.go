// Package handler содержит бенчмарки для HTTP-обработчиков сервиса сокращения URL.
//
// Бенчмарки измеряют производительность ключевых операций:
//   - Создание новой короткой ссылки
//   - Повторное сокращение уже существующей ссылки
//   - Редирект по существующему короткому URL
//   - Редирект по несуществующему URL
//   - Пакетное сокращение ссылок
//
// Все бенчмарки используют реальный MemoryRepository и NewHandler для максимальной реалистичности.
package handler

import (
	auditService "github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	URLService "github.com/heavydash/my-url-shortenergo/internal/service"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// BenchmarkShortenNewURL измеряет производительность создания новой короткой ссылки.
//
// Сценарий: каждый вызов бенчмарка создаёт новую ссылку через JSON-эндпоинт.
// Используется для оценки overhead генерации ID, сохранения в память и формирования ответа.
func BenchmarkShortenNewURL(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)

	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	logger := zap.NewNop()
	auditNoop := &auditService.Noop{}

	h := NewHandler(svc, cfg, logger, auditNoop)

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()

	b.ResetTimer()

	for b.Loop() {
		w.Body.Reset()
		h.ShortenHandler(w, req, false)
	}
}

// BenchmarkShortenExistingURL измеряет производительность повторного сокращения уже существующей ссылки.
//
// Сценарий: сначала создаётся ссылка, затем многократно вызывается сокращение той же ссылки.
// Позволяет оценить путь "URL уже существует" (конфликт + возврат существующего короткого URL).
func BenchmarkShortenExistingURL(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)

	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &auditService.Noop{}

	h := NewHandler(svc, cfg, logger, auditNoop)

	body := strings.NewReader("https://example.com")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ShortenHandler(w, req, false)

	b.ResetTimer()

	for b.Loop() {
		w.Body.Reset()
		h.ShortenHandler(w, req, false)
	}
}

// BenchmarkResolveFound измеряет производительность редиректа по существующему короткому URL.
//
// Сценарий: быстрый путь — URL найден в хранилище, возвращается 307 Temporary Redirect.
func BenchmarkResolveFound(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)

	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &auditService.Noop{}

	h := NewHandler(svc, cfg, logger, auditNoop)

	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()

	for b.Loop() {
		w.Body.Reset()
		h.RedirectURL(w, req)
	}
}

// BenchmarkResolveNotFound измеряет производительность редиректа по несуществующему короткому URL.
//
// Сценарий: URL не найден → возвращается 404 Not Found.
// Позволяет оценить overhead поиска отсутствующей записи.
func BenchmarkResolveNotFound(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)

	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &auditService.Noop{}

	h := NewHandler(svc, cfg, logger, auditNoop)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent123", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()

	for b.Loop() {
		w.Body.Reset()
		h.RedirectURL(w, req)
	}
}

// BenchmarkBatchShorten измеряет производительность пакетного сокращения нескольких ссылок.
//
// Сценарий: обработка массива из 3 URL за один запрос (включая генерацию ID, сохранение и формирование ответа).
func BenchmarkBatchShorten(b *testing.B) {
	repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
	svc := URLService.NewURLService(repo)

	cfg := &config.Config{BaseURL: "http://localhost:8080/"}
	logger := zap.NewNop()
	auditNoop := &auditService.Noop{}

	h := NewHandler(svc, cfg, logger, auditNoop)

	body := strings.NewReader(`[
		{"correlation_id": "1", "original_url": "https://ya.ru"},
		{"correlation_id": "2", "original_url": "https://google.com"},
		{"correlation_id": "3", "original_url": "https://example.com"},
	]`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten/batch", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		h.BatchShortenHandler(w, req)
	}
}
