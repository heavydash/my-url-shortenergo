// Package handler предоставляет HTTP-обработчики для сервиса сокращения URL.
package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"go.uber.org/zap"
)

// ShortenPlainHandler обрабатывает запросы на сокращение URL в текстовом формате.
//
// Ожидает POST-запрос с телом, содержащим оригинальный URL.
// Возвращает сокращенный URL в текстовом виде.
//
// Пример запроса:
//
//	POST /
//	Body: https://example.com/very-long-url
//
// Пример ответа:
//
//	http://short.ly/abc123
//
// Коды ответа:
//   - 201 Created - URL успешно сокращен
//   - 400 Bad Request - некорректный URL или запрос
//   - 409 Conflict - URL уже был сокращен (возвращает существующий сокращенный URL)
//   - 500 Internal Server Error - внутренняя ошибка сервера
func (h *Handler) ShortenPlainHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, false)
}

// HomeHandler обрабатывает запросы к корневому пути ("/").
//
// Предоставляет простую информационную страницу о сервисе.
// Принимает только GET-запросы.
//
// Пример запроса:
//
//	GET /
//
// Пример ответа:
//
//	"URL Shortener Service - Use POST / to shorten and GET /{id} to redirect"
//
// Коды ответа:
//   - 200 OK - успешный ответ
//   - 405 Method Not Allowed - использован неподдерживаемый метод
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if method := r.Method; method != http.MethodGet {
		h.logger.Info("Method not allowed: %s", zap.String("method", method))
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := w.Write([]byte("URL Shortener Service - Use POST / to shorten and GET /{id} to redirect")); err != nil {
		h.logger.Error("Write home message failed", zap.Error(err))
	}
}

// RedirectURL обрабатывает перенаправление по сокращенному URL.
//
// Извлекает идентификатор из пути URL и выполняет перенаправление
// на оригинальный URL. Также отправляет аудит-событие о переходе.
//
// Пример запроса:
//
//	GET /abc123
//
// Пример ответа:
//
//	HTTP 307 Temporary Redirect на оригинальный URL
//
// Коды ответа:
//   - 307 Temporary Redirect - успешное перенаправление
//   - 400 Bad Request - отсутствует идентификатор в пути
//   - 404 Not Found - идентификатор не найден в базе данных
//   - 410 Gone - запрашиваемый URL был удален
//   - 500 Internal Server Error - внутренняя ошибка сервера
//
// Параметры:
//   - w: http.ResponseWriter для записи ответа
//   - r: *http.Request входящий HTTP-запрос
func (h *Handler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Redirect handler called", zap.String("path", r.URL.Path))

	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	urlModel, err := h.repo.GetURL(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Error("Error finding URL for ID %s: %v", zap.String("id", id), zap.Error(err))
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			log.Printf("Error finding URL for ID %s: %v", id, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	h.logger.Info("IsDeleted", zap.Bool("deleted", urlModel.IsDeleted))
	if urlModel.IsDeleted {
		http.Error(w, "Gone", http.StatusGone)
		return
	}

	// Добавляем аудит
	userIDstr := ""
	if userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID); ok && userID != uuid.Nil {
		userIDstr = userID.String()
	}
	h.auditSvc.SendAsync(audit.NewFollowEvent(userIDstr, urlModel.OriginalURL))

	http.Redirect(w, r, urlModel.OriginalURL, http.StatusTemporaryRedirect)
}

// PingHandler проверяет доступность базы данных.
//
// Используется для health check'ов и мониторинга.
// Проверяет соединение с базой данных и возвращает статус.
//
// Пример запроса:
//
//	GET /ping
//
// Пример ответа:
//
//	"OK" (если БД доступна)
//
// Коды ответа:
//   - 200 OK - база данных доступна
//   - 500 Internal Server Error - ошибка соединения с БД
//
// Примечание:
//
//	Этот эндпоинт полезен для оркестраторов (Kubernetes, Docker Swarm)
//	и систем мониторинга для проверки готовности сервиса.
func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Ping(r.Context()); err != nil {
		h.logger.Error("DB ping failed", zap.Error(err))
		http.Error(w, "db ping failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		h.logger.Error("write string failed", zap.Error(err))
	}
}
