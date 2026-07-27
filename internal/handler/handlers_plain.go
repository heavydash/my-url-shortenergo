// Package handler предоставляет HTTP-обработчики для сервиса сокращения URL.
package handler

import (
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
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

// ShortenPlainHandler godoc
// @Summary      Создать короткую ссылку (plain text)
// @Description  Принимает URL в виде обычного текста (не JSON) в теле запроса.
//
//	Используется для POST /
//
// @Tags         urls
// @Accept       plain
// @Produce      json
// @Param        body  body      string  true  "URL для сокращения"  example:"https://www.google.com"
// @Success      201  {object}  api.ShortenResponse
// @Success      409  {object}  api.ShortenResponse
// @Failure      400  {object}  api.ErrorResponse
// @Failure      401  {object}  api.ErrorResponse
// @Failure      500  {object}  api.ErrorResponse
// @Router       / [post]
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

// HomeHandler godoc
// @Summary      Главная страница сервиса
// @Description  Возвращает простое информационное сообщение о сервисе.
//
//	Используется как приветственная страница по корневому пути "/".
//
// @Tags         general
// @Produce      plain
// @Success      200  {string}  string  "URL Shortener Service - Use POST / to shorten and GET /{id} to redirect"
// @Failure      405  {object}  api.ErrorResponse  "Метод не разрешён (разрешён только GET)"
// @Router       / [get]
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

// RedirectURL godoc
// @Summary      Редирект по короткой ссылке
// @Description  Выполняет перенаправление (HTTP 307 Temporary Redirect) на оригинальный URL.
//
//	**Важно:** При нажатии "Execute" в Swagger UI может показаться "Undocumented" или "Failed to fetch" —
//	это нормальное поведение Swagger, потому что он не следует за редиректами.
//	В реальном использовании переход по короткой ссылке работает корректно.
//
// @Tags         urls
// @Produce      plain
// @Param        id   path      string  true  "Короткий идентификатор ссылки"  example("4z31jnFH")
// @Success      307  "Temporary Redirect на оригинальный URL"
// @Header       307  Location  string  "Оригинальный URL"  example("https://www.amazon.com")
// @Failure      400  {object}  api.ErrorResponse
// @Failure      404  {object}  api.ErrorResponse
// @Failure      410  {object}  api.ErrorResponse
// @Failure      500  {object}  api.ErrorResponse
// @Router       /{id} [get]
func (h *Handler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Redirect handler called", zap.String("path", r.URL.Path))

	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	urlModel, err := h.service.GetURL(r.Context(), id)
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
	userID := middleware.GetUserID(r.Context())
	userIDstr := ""
	if userID != uuid.Nil {
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

// PingHandler godoc
// @Summary      Проверка работоспособности сервиса
// @Description  Возвращает "OK", если сервер и подключение к базе данных работают корректно.
//
//	Используется для health checks в мониторинге, Kubernetes, Docker и т.д.
//
// @Tags         health
// @Produce      plain
// @Success      200  {string}  string  "OK"
// @Failure      500  {string}  string  "db ping failed"
// @Router       /ping [get]
func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Ping(r.Context()); err != nil {
		h.logger.Error("DB ping failed", zap.Error(err))
		http.Error(w, "db ping failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		h.logger.Error("write string failed", zap.Error(err))
	}
}
