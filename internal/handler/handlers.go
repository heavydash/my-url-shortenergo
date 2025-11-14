package handler

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/model"
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
	r.Post("/", h.ShortenURL)
	r.Post("/api/shorten", h.ShortenURL)
	r.Get("/", h.HomeHandler)
	r.Get("/{id}", h.RedirectURL)

}

func (h *Handler) ShortenURL(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed reading body: %v", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var url string

	isAPI := strings.HasPrefix(r.URL.Path, "/api/shorten")
	isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

	if isAPI && isJSON {
		req := model.Request{}
		if err := req.UnmarshalJSON(body); err != nil {
			h.logger.Error("Failed unmarshalling body: %v", zap.Error(err))
			resp := model.Response{Error: "Invalid JSON body"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			data, err := resp.MarshalJSON()
			if err != nil {
				h.logger.Error("Failed writing body: %v", zap.Error(err))
				http.Error(w, "Failed writing body", http.StatusInternalServerError)
				return
			}
			w.Write(data)
			return
		}
		url = req.URL
	} else {
		url = string(body)
	}

	if len(url) == 0 || !strings.HasPrefix(url, "http") {
		if isAPI && isJSON {
			h.logger.Info("Invalid URL: %v", zap.String("url", url))
			resp := model.Response{Error: "Invalid URL"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			data, err := resp.MarshalJSON()
			if err != nil {
				h.logger.Error("Failed writing body: %v", zap.Error(err))
				http.Error(w, "Failed writing body", http.StatusInternalServerError)
				return
			}
			w.Write(data)
			return
		} else {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
	}
	urlModel := model.URLModel{
		OriginalURL: url,
	}
	savedModel, err := h.repo.SaveURL(urlModel)
	if err != nil {
		h.logger.Error("Failed saving URL: %v", zap.Error(err))
		if err.Error() == "failed to generate short ID after 10 attempts" {
			if isAPI && isJSON {
				resp := model.Response{Error: "Server overloaded, try again later"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				data, err := resp.MarshalJSON()
				if err != nil {
					h.logger.Error("Failed writing body: %v", zap.Error(err))
					http.Error(w, "Failed writing body", http.StatusInternalServerError)
					return
				}
				w.Write(data)
			} else {
				http.Error(w, "Server overloaded, try again later", http.StatusServiceUnavailable)
				return
			}
		} else {
			if isAPI && isJSON {
				resp := model.Response{Error: "Internal server error"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				data, err := resp.MarshalJSON()
				if err != nil {
					h.logger.Error("Failed writing body: %v", zap.Error(err))
					http.Error(w, "Failed writing body", http.StatusInternalServerError)
					return
				}
				w.Write(data)
				return
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
	}
	shortURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), savedModel.UUID)
	if isAPI && isJSON {
		resp := model.Response{Result: shortURL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		data, err := resp.MarshalJSON()
		if err != nil {
			h.logger.Error("Failed writing body: %v", zap.Error(err))
			http.Error(w, "Failed writing body", http.StatusInternalServerError)
			return
		}
		w.Write(data)
		return
	} else {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(shortURL))
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

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
