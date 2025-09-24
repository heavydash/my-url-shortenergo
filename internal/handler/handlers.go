package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

type Handler struct {
	repo repository.URLRepository
}

func NewHandler(repo *repository.MemoryRepository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", 400)
			return
		}
		urlString := string(body)
		if len(urlString) == 0 || !strings.HasPrefix(urlString, "http") {
			http.Error(w, "Invalid URL", 400)
			return
		}
		urlmodel := model.URLModel{URL: urlString}
		savedModel, err := h.repo.SaveURL(urlmodel)
		if err != nil {
			http.Error(w, "Failed to save URL", 400)
			return
		}
		w.Header().Set("content-type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write([]byte("http://localhost:8080/" + savedModel.ID))
		if err != nil {
			http.Error(w, "Write error", 500)
			return
		}
	} else if r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/")
		if len(id) == 0 {
			http.Error(w, "Invalid ID", 400)
			return
		}
		urlmodel, err := h.repo.GetURL(id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "ID not found", 400)
			} else {
				http.Error(w, "Method not allowed", 500)
			}
			return
		}
		http.Redirect(w, r, urlmodel.URL, http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "Method not allowed", 405)
		return
	}
}
