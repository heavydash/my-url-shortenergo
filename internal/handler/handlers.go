package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

type Handler struct {
	repo repository.URLRepository
}

func NewHandler(repo repository.URLRepository) *Handler {
	return &Handler{repo: repo}

}

func (h *Handler) SetupRoutes(r *chi.Mux) {
	r.Post("/", h.ShortenURL)
	r.Get("/{id}", h.RedirectURL)
}

func (h *Handler) ShortenURL(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", 400)
		return
	}
	url := string(body)
	if len(url) == 0 || !strings.HasPrefix(url, "http") {
		http.Error(w, "Invalid URL", 400)
		return
	}
	urlModel := model.URLModel{URL: url}
	savedModel, err := h.repo.SaveURL(urlModel)
	if err != nil {
		http.Error(w, "Failed to save URL", 500)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("http://localhost:8080/" + savedModel.ID))
}

func (h *Handler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if len(id) == 0 {
		http.Error(w, "Invalid ID", 400)
		return
	}
	urlModel, err := h.repo.GetURL(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "ID not found", 404)
		} else {
			http.Error(w, "Internal server error", 500)
		}
		return
	}
	http.Redirect(w, r, urlModel.URL, http.StatusTemporaryRedirect)
}
