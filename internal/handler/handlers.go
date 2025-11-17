package handler

import (
	"encoding/json"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

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

	//Парсинг запроса
	reqUrl, err := h.parseRequestBody(r, isJSON)
	if err != nil {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}

	//Валидация URL
	if !h.isValidURL(reqUrl) {
		h.sendError(w, isJSON, "Invalid request", http.StatusBadRequest)
		return
	}

	//Генерация ID
	id, err := idgen.IDGen()
	if err != nil || id == "" {
		h.logger.Error("Failed to generate ID", zap.Error(err))
		h.sendError(w, isJSON, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	//Модель
	m := model.URLModel{
		UUID:        id,
		ShortURL:    fmt.Sprintf("%s%s", strings.TrimRight(h.cfg.BaseURL, "/"), id),
		OriginalURL: reqUrl,
	}

	//Сохранение
	saved, err := h.repo.SaveURL(m)
	if err != nil {
		h.logger.Error("Failed to save URL", zap.Error(err))
		status := http.StatusInternalServerError
		msg := "Internal Server Error"
		if strings.Contains(err.Error(), "collision") {
			status = http.StatusServiceUnavailable
			msg = "Service overloaded, try again later"
		}
		h.sendError(w, isJSON, msg, status)
		return
	}
	//Отправка ответа
	h.sendSuccess(w, isJSON, saved.ShortURL)
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

func (h *Handler) sendSuccess(w http.ResponseWriter, isJSON bool, shortURL string) {
	if isJSON {
		resp := model.Response{Result: shortURL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	} else {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(shortURL))
	}
}
func (h *Handler) sendError(w http.ResponseWriter, isJSON bool, msg string, status int) {
	if isJSON {
		resp := model.Response{Error: msg}
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
	w.Write([]byte("URL Shortener Service - Use POST / to shorten and GET /{id} to redirect"))
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
	w.Write([]byte("OK"))
}
