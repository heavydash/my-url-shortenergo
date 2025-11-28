package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"go.uber.org/zap"

	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

type Handler struct {
	repo   repository.URLRepository
	cfg    *config.Config
	logger *zap.Logger
}

func NewHandler(
	repo repository.URLRepository,
	cfg *config.Config,
	logger *zap.Logger) *Handler {
	return &Handler{
		repo:   repo,
		cfg:    cfg,
		logger: logger}
}

func (h *Handler) SetupRoutes(r *chi.Mux) {
	r.Get("/ping", h.PingHandler)
	r.Post("/", h.ShortenPlainHandler)
	r.Post("/api/shorten", h.ShortenJSONHandler)
	r.Post("/api/shorten/batch", h.BatchShortenHandler)
	r.Get("/", h.HomeHandler)
	r.Get("/{id}", h.RedirectURL)

}

func (h *Handler) ShortenJSONHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, true)
}

func (h *Handler) ShortenPlainHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, false)
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
	userID, _ := r.Context().Value("userID").(string)
	h.logger.Info("ShortenHandler: userID", zap.String("user_id", userID))

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
		zap.String("user_id", m.UserID))

	//Сохранение
	saved, err := h.repo.SaveURL(m)
	if err != nil {
		h.logger.Error("ShortenHandler: SaveURL failed", zap.Error(err))
		if errors.Is(err, repository.ErrConflict) {
			fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), saved.ShortURL)
			w.WriteHeader(http.StatusConflict)
			h.sendResponse(w, isJSON, fullURL, http.StatusConflict)
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
	w.WriteHeader(http.StatusCreated)
	h.sendResponse(w, isJSON, fullURL, http.StatusCreated)
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

func (h *Handler) sendResponse(w http.ResponseWriter, isJSON bool, shortURL string, status int) {
	if isJSON {
		resp := model.Response{Result: shortURL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	w.Write([]byte(shortURL))
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
func (h *Handler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if len(id) == 0 {
		h.logger.Error("Error invalid ID: empty", zap.String("id", id))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
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
	http.Redirect(w, r, urlModel.OriginalURL, http.StatusTemporaryRedirect)
}
func (h *Handler) PingHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Ping(r.Context()); err != nil {
		h.logger.Error("DB ping failed", zap.Error(err))
		http.Error(w, "db ping failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		h.logger.Error("writestring failed", zap.Error(err))
	}
}
func (h *Handler) BatchShortenHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		userID = "anonymous"
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
			UUID:        id,
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
