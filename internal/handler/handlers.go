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
	r.Post("/api/shorten", h.ShortenURL)
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
	var url string

	isAPI := strings.HasPrefix(r.URL.Path, "/api/shorten")
	isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

	if isAPI && isJSON {
		req := model.Request{}
		if err := req.UnmarshalJSON(body); err != nil {
			log.Printf("Failed unmarshalling body: %v", err)
			resp := model.Response{Error: "Invalid JSON body"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			data, err := resp.MarshalJSON()
			if err != nil {
				log.Printf("Failed writing body: %v", err)
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
			log.Printf("Invalid URL: %v", url)
			resp := model.Response{Error: "Invalid URL"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			data, err := resp.MarshalJSON()
			if err != nil {
				log.Printf("Failed writing body: %v", err)
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
		URL: url,
	}
	savedModel, err := h.repo.SaveURL(urlModel)
	if err != nil {
		log.Printf("Failed saving URL: %v", err)
		if err.Error() == "failed to generate short ID after 10 attempts" {
			if isAPI && isJSON {
				resp := model.Response{Error: "Server overloaded, try again later"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				data, err := resp.MarshalJSON()
				if err != nil {
					log.Printf("Failed writing body: %v", err)
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
					log.Printf("Failed writing body: %v", err)
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
	shortURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), savedModel.ID)
	if isAPI && isJSON {
		resp := model.Response{Result: shortURL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		data, err := resp.MarshalJSON()
		if err != nil {
			log.Printf("Failed writing body: %v", err)
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
