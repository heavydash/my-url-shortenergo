// Package handler предоставляет HTTP-обработчики для сервиса сокращения URL.
//
// Основные эндпоинты:
//   - POST /           - сокращение URL в plain text формате
//   - POST /api/shorten - сокращение URL в JSON формате
//   - GET  /{id}       - редирект по короткому URL
//   - GET  /ping       - health check сервиса
//   - GET  /           - главная страница с документацией
//   - POST /api/shorten/batch - пакетное сокращение
//   - GET  /api/user/urls     - список URL пользователя
//   - DELETE /api/user/urls   - пометка URL на удаление
//
// Все эндпоинты поддерживают gzip-сжатие и логирование через middleware.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	auditservice "github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	urlservice "github.com/heavydash/my-url-shortenergo/internal/service"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/deleter"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/util"

	"go.uber.org/zap"
)

// Handler содержит зависимости для обработки HTTP запросов сервиса сокращения URL.
// Обрабатывает создание коротких URL, редиректы, пакетные операции и управление пользовательскими данными.
//
// Поля:
//
//   - service  - бизнес-слой (URLService)
//     cfg      - конфигурация приложения (базовый URL, настройки удаления)
//     logger   - логгер для записи событий
//     deleter  - асинхронный обработчик удаления URL
//     auditSvc - сервис аудита для отслеживания операций
type Handler struct {
	service  urlservice.URLService
	cfg      *config.Config
	logger   *zap.Logger
	deleter  *deleter.URLDeleter
	auditSvc auditservice.Service
}

// NewHandler создает новый экземпляр HTTP обработчика со всеми зависимостями.
//
// Параметры:
//   - service  - бизнес-слой (URLService)
//   - cfg      - конфигурация приложения (может быть nil в тестах)
//   - logger   - логгер (если nil — будет создан no-op)
//   - auditSvc - сервис аудита (может быть nil)
//
// Возвращает готовый Handler с инициализированным deleter.
func NewHandler(
	service urlservice.URLService,
	cfg *config.Config,
	logger *zap.Logger,
	auditSvc service.Service,
) *Handler {
	effectiveLogger := logger
	if effectiveLogger == nil {
		effectiveLogger = zap.NewNop()
	}

	// создаем Deleter для мягкого удаления URL
	del := deleter.NewURLDeleter(
		service,
		effectiveLogger,
		cfg.DeletionQueueBuffer,
		cfg.DeletionFlushInterval,
		cfg.DeletionMaxBatchSize,
	)

	return &Handler{
		service:  service,
		cfg:      cfg,
		logger:   effectiveLogger,
		deleter:  del,
		auditSvc: auditSvc,
	}
}

// ShortenHandler — внутренний метод, который используется ShortenPlainHandler и ShortenJSONHandler.
// Выполняет основную логику сокращения одной ссылки.
//
// Параметры:
//   - w      - ResponseWriter для записи ответа
//   - r      - Request с телом запроса
//   - isJSON - true для JSON формата, false для plain text
//
// Логика работы:
//  1. Парсит URL из тела запроса (JSON или plain text)
//  2. Валидирует формат URL
//  3. Получает или создаёт userID из cookies
//  4. Генерирует уникальный короткий ID
//  5. Сохраняет запись в репозиторий
//  6. Возвращает полный сокращённый URL клиенту
//
// Коды ответа:
//   - 201 Created - URL успешно сокращён
//   - 400 Bad Request - невалидный URL или тело запроса
//   - 409 Conflict - URL уже существует (возвращает существующий короткий URL)
//   - 500 Internal Server Error - ошибка генерации ID или сохранения
func (h *Handler) ShortenHandler(w http.ResponseWriter, r *http.Request, isJSON bool) {
	h.logger.Info("ShortenHandler: request started", zap.Bool("isJSON", isJSON))

	h.logger.Debug("ShortenHandler: parsing request body")
	// Парсинг тела запроса
	reqURL, err := h.parseRequestBody(r, isJSON)
	if err != nil {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}
	h.logger.Debug("ShortenHandler: parsed URL", zap.String("original_url", reqURL))

	// Валидация URL
	if !h.isValidURL(reqURL) {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}

	// Получаем userID или создаём userID
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	if !ok || userID == uuid.Nil {
		userID = uuid.New()
		util.SetSignedCookie(w, userID)
	}

	h.logger.Debug("ShortenHandler: userID", zap.String("user_id", userID.String()))

	// Генерация короткого ID
	id, err := idgen.IDGen()
	if err != nil || id == "" {
		h.logger.Error("Failed to generate ID", zap.Error(err))
		h.sendError(w, isJSON, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.logger.Debug("ShortenHandler: generated ID", zap.String("id", id))

	// Создаём модель для сохранения
	m := model.URLModel{
		UUID:        id,
		ShortURL:    id,
		OriginalURL: reqURL,
		UserID:      userID,
	}

	h.logger.Debug("ShortenHandler: calling SaveURL",
		zap.String("uuid", m.UUID),
		zap.String("short_url", m.ShortURL),
		zap.String("original_url", m.OriginalURL),
		zap.String("user_id", userID.String()))

	// Сохраняем в репозиторий
	saved, err := h.service.SaveURL(r.Context(), m)
	if err != nil {
		h.logger.Error("ShortenHandler: SaveURL failed", zap.Error(err))
		if errors.Is(err, repository.ErrConflict) {
			fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), saved.ShortURL)
			h.logger.Info("ShortenHandler: URL saved successfully", zap.String("short_url", fullURL))
			if isJSON {
				h.sendResponse(w, isJSON, model.Response{Result: fullURL}, http.StatusConflict)
			} else {
				h.sendResponse(w, isJSON, fullURL, http.StatusConflict)
			}
			return
		}
		// 500
		h.sendError(w, isJSON, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Формируем полный короткий URL при  201
	fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"),
		saved.ShortURL)
	h.logger.Info("ShortenHandler: URL saved successfully", zap.String("short_url",
		fullURL))

	// Добавляем событие в аудит
	userIDStr := ""
	if userID != uuid.Nil {
		userIDStr = userID.String()
	}
	h.auditSvc.SendAsync(audit.NewShortenEvent(userIDStr, reqURL))

	// Отправляем ответ клиенту
	if isJSON {
		h.sendResponse(w, isJSON, model.Response{Result: fullURL}, http.StatusCreated)
	} else {
		h.sendResponse(w, isJSON, fullURL, http.StatusCreated)
	}
}

// parseRequestBody парсит тело HTTP запроса в зависимости от формата (JSON или plain text).
// Используется только внутри ShortenHandler.
//
// Возвращает:
//   - string - распарсенный URL
//   - error  - ошибка парсинга или чтения тела
func (h *Handler) parseRequestBody(r *http.Request, isJSON bool) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read body", zap.Error(err))
		return "", err
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			h.logger.Error("Failed to close body", zap.Error(err))
		}
	}()

	if isJSON {
		req := model.Request{}
		if err := json.Unmarshal(body, &req); err != nil {
			return "", err
		}
		return req.URL, nil
	}
	return string(body), nil
}

// isValidURL проверяет, что строка является валидным URL с http/https схемой.
// Используется только внутри ShortenHandler.
//
// Возвращает:
//   - bool - true если URL валиден, false в противном случае
func (h *Handler) isValidURL(u string) bool {
	if u == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
		return false
	}
	_, err := url.ParseRequestURI(u)
	return err == nil
}

// sendResponse отправляет успешный ответ в нужном формате (JSON или plain text).
// Используется внутри хендлеров.
func (h *Handler) sendResponse(w http.ResponseWriter, isJSON bool, data any, status int) {
	if isJSON {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		// Теперь any можно передать строку, слайс, мпау, структуру
		if err := json.NewEncoder(w).Encode(data); err != nil {
			h.logger.Error("Failed to encode response", zap.Error(err))
		}
		return
	}

	str, ok := data.(string)
	if !ok {
		http.Error(w, "InternalServerError", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(str))
}

// sendError отправляет ответ с ошибкой в нужном формате (JSON или plain text).
// Используется внутри хендлеров.
func (h *Handler) sendError(w http.ResponseWriter, isJSON bool, msg string, status int) {
	if isJSON {
		resp := map[string]string{"error": msg}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			h.logger.Error("Failed to encode response", zap.Error(err))
		}
	} else {
		http.Error(w, msg, status)
	}
}

// Close освобождает ресурсы обработчика, останавливая deleter.
// Должен вызываться при завершении работы приложения.
func (h *Handler) Close() error {
	if h.deleter != nil {
		return h.deleter.Close()
	}
	return nil
}
