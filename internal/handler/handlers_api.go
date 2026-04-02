package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/deleter"
	"github.com/heavydash/my-url-shortenergo/internal/generator"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"go.uber.org/zap"
)

// ShortenJSONHandler обрабатывает POST запросы с JSON телом для сокращения URL.
//
// Пример запроса:
//
//	POST /api/shorten
//	Content-Type: application/json
//	Body: {"url": "https://example.com"}
//
// Пример ответа:
//
//	201 Created
//	Content-Type: application/json
//	Body: {"result": "http://localhost:8080/abc123"}
//
// Если URL уже существует, возвращает 409 Conflict с существующим сокращенным URL.
//
// Коды ответа:
//
//	201 Created - URL успешно сокращен
//	400 Bad Request - невалидный JSON или URL
//	409 Conflict - URL уже существует
//	500 Internal Server Error - ошибка сервера
func (h *Handler) ShortenJSONHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, true)
}

// GetUserURLs возвращает все сокращенные URL, созданные текущим пользователем.
//
// Пример запроса:
//
//	GET /api/user/urls
//	Cookie: user_id=<signed-user-uuid>
//
// Пример ответа:
//
//	200 OK
//	Content-Type: application/json
//	Body: [{"short_url":"http://localhost:8080/abc123","original_url":"https://example.com"}]
//
// Если у пользователя нет URL, возвращает 204 No Content.
// Требует аутентификации через middleware.Auth.
//
// Коды ответа:
//
//	200 OK - успешно возвращены URL пользователя
//	204 No Content - у пользователя нет сохраненных URL
//	401 Unauthorized - отсутствует или невалидный user_id в cookies
//	500 Internal Server Error - ошибка при получении данных
func (h *Handler) GetUserURLs(w http.ResponseWriter, r *http.Request) {
	// Достаём из контекста
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Дополнительная проверка: Если куки изначально не было — middleware создал нового. Для /api/user/urls это не подходит.
	originalCookie, _ := r.Cookie("user_id")
	if originalCookie == nil {
		// Куки не было изначально - новый пользователь
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Теперь ищем URL этого пользователя
	urls, err := h.service.GetURLsByUser(context.Background(), userID)
	if err != nil {
		h.logger.Error("GetURLsByUser failed", zap.Error(err))
		h.sendError(w, true, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Если URL нет — 204
	if len(urls) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.sendResponse(w, true, urls, http.StatusOK)
}

// BatchShortenHandler обрабатывает пакетное создание нескольких коротких URL за один запрос.
//
// Пример запроса:
//
//	POST /api/shorten/batch
//	Content-Type: application/json
//	Body: [
//	  {"correlation_id": "1", "original_url": "https://example.com/1"},
//	  {"correlation_id": "2", "original_url": "https://example.com/2"}
//	]
//
// Пример ответа:
//
//	201 Created
//	Content-Type: application/json
//	Body: [
//	  {"correlation_id": "1", "short_url": "http://localhost:8080/abc123"},
//	  {"correlation_id": "2", "short_url": "http://localhost:8080/def456"}
//	]
//
// Валидация:
//   - correlation_id должен быть уникальным в пределах запроса
//   - URL должен иметь схему http:// или https://
//   - Batch не может быть пустым
//
// Коды ответа:
//
//	201 Created - пакет успешно обработан
//	400 Bad Request - невалидный JSON, дубликаты correlation_id, пустой batch
//	500 Internal Server Error - ошибка при сохранении
func (h *Handler) BatchShortenHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		userID = uuid.New()
		util.SetSignedCookie(w, userID)
	}

	if r.Method != http.MethodPost {
		h.sendError(w, true, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var batch []model.BatchRequestItem
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		h.logger.Error("json decode failed", zap.Error(err))
		h.sendError(w, true, "invalid json", http.StatusBadRequest)
		return
	}
	if len(batch) == 0 {
		h.sendError(w, true, "empty batch", http.StatusBadRequest)
		return
	}
	// Дублирование correlation_id
	seen := make(map[string]bool)
	for _, item := range batch {
		if seen[item.CorrelationID] {
			h.sendError(w, true, "duplicate correlation_id", http.StatusBadRequest)
			return
		}
		seen[item.CorrelationID] = true
		if item.CorrelationID == "" {
			h.sendError(w, true, "empty original_url", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(item.OriginalURL, "http://") && !strings.HasPrefix(item.OriginalURL, "https://") {
			h.sendError(w, true, "invalid url scheme", http.StatusBadRequest)
			return
		}
	}

	// Генерация моделей
	batchModels := make([]model.URLModel, 0, len(batch))
	respMap := make(map[string]string)

	for _, item := range batch {
		id, err := generator.IDGen()
		if err != nil || id == "" {
			h.logger.Error("Failed to generate ID", zap.Error(err))
			h.sendError(w, true, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		shortURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), id)

		batchModels = append(batchModels, model.URLModel{
			ShortURL:    id,
			OriginalURL: item.OriginalURL,
			UserID:      userID,
		})

		respMap[item.CorrelationID] = shortURL
	}
	// Сохранение batch
	if err := h.service.SaveBatch(ctx, batchModels); err != nil {
		h.logger.Error("Failed to save batch", zap.Error(err))
		h.sendError(w, true, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Формирование ответа
	batchResp := make([]model.BatchResponseItem, 0, len(batch))
	for _, item := range batch {
		batchResp = append(batchResp, model.BatchResponseItem{
			CorrelationID: item.CorrelationID,
			OriginalURL:   respMap[item.CorrelationID],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(batchResp); err != nil {
		h.logger.Error("encode batch response failed", zap.Error(err))
	}
}

// DeleteUrls помечает указанные короткие URL для асинхронного удаления.
// Удаление выполняется фоном через URLDeleter, метод сразу возвращает 202 Accepted.
//
// Пример запроса:
//
//	DELETE /api/user/urls
//	Content-Type: application/json
//	Cookie: user_id=<signed-user-uuid>
//	Body: ["abc123", "def456"]
//
// Пример ответа:
//
//	202 Accepted
//
// Особенности:
//   - Удаление только своих URL (проверяется по user_id)
//   - Soft delete (URL помечаются как удаленные, но остаются в БД)
//   - Асинхронная обработка, запрос сразу возвращает 202
//   - Автоматическая дедупликация ID в запросе
//   - Требует аутентификации через middleware.Auth
//
// Коды ответа:
//
//	202 Accepted - запрос на удаление принят в обработку
//	400 Bad Request - невалидный JSON
//	401 Unauthorized - отсутствует или невалидный user_id в cookies
func (h *Handler) DeleteUrls(w http.ResponseWriter, r *http.Request) {

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		h.sendError(w, true, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Дополнительная проверка: была ли кука изначально?
	// Если куки не было — middleware создал нового пользователя.
	// Для удаления это не подходит.
	originalCookie, _ := r.Cookie("user_id")
	if originalCookie == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		h.sendError(w, true, "Bad request", http.StatusBadRequest)
		return
	}
	h.logger.Info("DeleteUrls: received request", zap.Int("raw_ids_count", len(ids)))

	h.logger.Info("DeleteUrls: ids", zap.Strings("ids", ids))

	if len(ids) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Дедупликация ID
	unique := make(map[string]bool)
	var deduped []string
	for _, id := range ids {
		if !unique[id] {
			unique[id] = true
			deduped = append(deduped, id)
		}
	}
	// Отправляем задачу в Deleter
	h.deleter.Submit(deleter.DeletionTask{
		UserID:   userID,
		ShortIDs: deduped,
	})

	w.WriteHeader(http.StatusAccepted)
}

// GetInternalStats обрабатывает GET /api/internal/stats
//
// Возвращает JSON с общей статистикой сервиса:
//
//	{
//	  "urls":  12345,
//	  "users": 987
//	}
//
// Доступ разрешён **только** из доверенной подсети, указанной в конфигурации (TrustedSubnetNet).
// Проверка выполняется по заголовку X-Real-IP.
//
// Примеры ответов:
//
//	200 OK          - статистика успешно возвращена
//	403 Forbidden   - запрос не из доверенной подсети
//
// Использует репозиторий через вызов repo.Stats().
func (h *Handler) GetInternalStats(w http.ResponseWriter, r *http.Request) {
	// Проверка доступа из доверенной подсети
	if !h.isRequestFromTrustedSubnet(r) {
		h.logger.Warn("GetInternalStats: access denied: not trusted subnet IP",
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("x_real_ip", r.Header.Get("X-Real-IP")))

		h.sendError(w, true, "Forbidden", http.StatusForbidden)
		return

	}

	// Получаем статистику
	urls, users := h.service.Stats()

	h.logger.Info("GetInternalStats: success",
		zap.Int("urls", urls),
		zap.Int("users", users))

	// Формируем ответ
	response := struct {
		URLs  int `json:"urls"`
		Users int `json:"users"`
	}{
		URLs:  urls,
		Users: users,
	}
	// Успех, отправляем ответ
	h.sendResponse(w, true, response, http.StatusOK)
}

// isRequestFromTrustedSubnet проверяет, пришёл ли HTTP-запрос из доверенной подсети.
//
// Логика проверки (строго по порядку):
//  1. Если в конфигурации не задана TrustedSubnetNet (nil) → возвращает false.
//  2. Если в заголовке запроса отсутствует X-Real-IP → возвращает false.
//  3. Если значение X-Real-IP не удаётся распарсить как корректный IP → возвращает false
//     и логирует предупреждение.
//  4. Если IP входит в подсеть TrustedSubnetNet → возвращает true.
//
// Примечание:
//
//	Метод не выполняет никаких side-effect'ов, кроме логирования некорректного IP.
//	Используется исключительно в GetInternalStats.
func (h *Handler) isRequestFromTrustedSubnet(r *http.Request) bool {
	// Проверка конфигурации
	if h.cfg == nil || h.cfg.TrustedSubnetNet == nil {
		return false
	}

	// Извлекаем IP адрес
	ipStr := r.Header.Get("X-Real-IP")
	if ipStr == "" {
		return false
	}

	// Парсим IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		h.logger.Warn("isRequestFromTrustedSubnet: invalid IP in X-Real-IP header",
			zap.String("ip", ipStr))
		return false
	}

	// Проверка принадлежности подсети
	return h.cfg.TrustedSubnetNet.Contains(ip)
}
