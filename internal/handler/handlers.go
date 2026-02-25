package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/deleter"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/internal/util"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
)

type Handler struct {
	repo     repository.URLRepository
	cfg      *config.Config
	logger   *zap.Logger
	deleter  *deleter.URLDeleter
	auditSvc service.Service
}

func NewHandler(
	repo repository.URLRepository,
	cfg *config.Config,
	logger *zap.Logger,
	auditSvc service.Service,
) *Handler {
	effectiveLogger := logger
	if effectiveLogger == nil {
		effectiveLogger = zap.NewNop()
	}

	// Параметры Deleter
	bufferSize := 1000
	flushInterval := 500 * time.Millisecond
	maxBatchSize := 1000

	if cfg != nil {
		bufferSize = cfg.DeletionQueueBuffer
		if cfg.DeletionFlushInterval > 0 {
			flushInterval = cfg.DeletionFlushInterval
		} else {
			effectiveLogger.Warn("DeletionFlushInterval invalid or zero, using default 500ms")
		}
		maxBatchSize = cfg.DeletionMaxBatchSize
	} else {
		effectiveLogger.Info("Config is nil (likely in tests), using default Deleter params")
	}

	// создаем Deleter
	del := deleter.NewURLDeleter(
		repo,
		effectiveLogger,
		bufferSize,
		flushInterval,
		maxBatchSize,
	)

	return &Handler{
		repo:     repo,
		cfg:      cfg,
		logger:   effectiveLogger,
		deleter:  del,
		auditSvc: auditSvc,
	}
}

func (h *Handler) ShortenHandler(w http.ResponseWriter, r *http.Request, isJSON bool) {
	h.logger.Info("ShortenHandler: request started", zap.Bool("isJSON", isJSON))
	//Парсинг запроса
	reqURL, err := h.parseRequestBody(r, isJSON)
	if err != nil {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}
	h.logger.Info("ShortenHandler: parsed URL", zap.String("original_url", reqURL))

	//Валидация URL
	if !h.isValidURL(reqURL) {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}

	// Получаем userID из контекста
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	if !ok || userID == uuid.Nil {
		userID = uuid.New()
		util.SetSignedCookie(w, userID)
	}

	h.logger.Info("ShortenHandler: userID", zap.String("user_id", userID.String()))

	//Генерация ID
	id, err := idgen.IDGen()
	if err != nil || id == "" {
		h.logger.Error("Failed to generate ID", zap.Error(err))
		h.sendError(w, isJSON, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	h.logger.Info("ShortenHandler: generated ID", zap.String("id", id))

	//Модель
	m := model.URLModel{
		UUID:        id,
		ShortURL:    id,
		OriginalURL: reqURL,
		UserID:      userID,
	}

	h.logger.Info("ShortenHandler: calling SaveURL",
		zap.String("uuid", m.UUID),
		zap.String("short_url", m.ShortURL),
		zap.String("original_url", m.OriginalURL),
		zap.String("user_id", userID.String()))

	//Сохранение
	saved, err := h.repo.SaveURL(m)
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
	// 201
	fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"),
		saved.ShortURL)
	h.logger.Info("ShortenHandler: URL saved successfully", zap.String("short_url",
		fullURL))

	// Добавляем аудит
	userIDStr := ""
	if userID != uuid.Nil {
		userIDStr = userID.String()
	}
	h.auditSvc.SendAsync(audit.NewShortenEvent(userIDStr, reqURL))

	if isJSON {
		h.sendResponse(w, isJSON, model.Response{Result: fullURL}, http.StatusCreated)
	} else {
		h.sendResponse(w, isJSON, fullURL, http.StatusCreated)
	}
}

func (h *Handler) parseRequestBody(r *http.Request, isJSON bool) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read body", zap.Error(err))
		return "", err
	}
	defer r.Body.Close()
	if isJSON {
		req := model.Request{}
		if err := json.Unmarshal(body, &req); err != nil {
			return "", err
		}
		return req.URL, nil
	}
	return string(body), nil
}

func (h *Handler) isValidURL(u string) bool {
	if u == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
		return false
	}
	_, err := url.ParseRequestURI(u)
	return err == nil
}

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

func (h *Handler) sendError(w http.ResponseWriter, isJSON bool, msg string, status int) {
	if isJSON {
		resp := map[string]string{"error": msg}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resp)
	} else {
		http.Error(w, msg, status)
	}
}

func (h *Handler) Close() error {
	if h.deleter != nil {
		return h.deleter.Close()
	}
	return nil
}
