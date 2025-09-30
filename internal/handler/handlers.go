package handler

import (
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

type Handler struct {
	repo repository.URLRepository
	cfg  *config.Config
}

func NewHandler(repo repository.URLRepository, cfg *config.Config) *Handler {
	return &Handler{repo: repo, cfg: cfg}

}

func (h *Handler) SetupRoutes(r *chi.Mux) {
	r.Post("/", h.ShortenURL)
	r.Get("/", h.HomeHandler)
	r.Get("/{id}", h.RedirectURL)
}

func (h *Handler) ShortenURL(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed reading body: %v", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	url := string(body)
	if len(url) == 0 || !strings.HasPrefix(url, "http") {
		log.Printf("Invalid URL: %v", url)
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	urlModel := model.URLModel{URL: url}
	savedModel, err := h.repo.SaveURL(urlModel)
	if err != nil {
		log.Printf("Error saving URL: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	shortURL := fmt.Sprintf("%s%s", strings.TrimRight(h.cfg.BaseURL, "/"), "/"+savedModel.ID)
	w.Write([]byte(shortURL))
}
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if method := r.Method; method != http.MethodGet {
		log.Printf("Method not allowed: %s", method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Write([]byte("URL Shortener Service - Use POST / to shorten and GET /{id} to redirect"))
}
func (h *Handler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if len(id) == 0 {
		log.Printf("Error invalid ID: empty")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	urlModel, err := h.repo.GetURL(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Printf("Error finding URL for ID %s: %v", id, err)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			log.Printf("Error finding URL for ID %s: %v", id, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, urlModel.URL, http.StatusTemporaryRedirect)
}
