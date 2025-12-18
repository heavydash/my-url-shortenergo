package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

func (h *Handler) ShortenJSONHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, true)
}

func (h *Handler) GetUserURLs(w http.ResponseWriter, r *http.Request) {
	// Достаём из контекста
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Теперь ищем URL этого пользователя
	urls, err := h.repo.GetURLsByUser(context.Background(), userID)
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
		id, err := idgen.IDGen()
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
	if err := h.repo.SaveBatch(ctx, batchModels); err != nil {
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

func (h *Handler) DeleteUrls(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		h.sendError(w, true, "Bad request", http.StatusBadRequest)
		return
	}

	if len(ids) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	go func() {
		if err := h.repo.MarkAsDeleted(userID, ids); err != nil {
			h.logger.Error("MarkAsDeleted failed in background", zap.Error(err))
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}
