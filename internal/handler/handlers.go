package handler

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"io"
	"log"
	"net/http"
	"strings"
)

type Handler struct {
	repo repository.URLRepository
	cfg  *config.Config
}

func NewHandler(repo repository.URLRepository, cfg *config.Config) *Handler {
	return &Handler{repo: repo, cfg: cfg}
}

func (h *Handler) SetupRoutes(r *chi.Mux) {
	r.Post("/api/shorten", h.ShortenURL)
	r.Post("/", h.ShortenURL)
	r.Get("/", h.HomeHandler)
	r.Get("/{id}", h.RedirectURL)
}

func (h *Handler) ShortenURL(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var url string
	isAPI := strings.HasPrefix(r.URL.Path, "/api/shorten")

	if isAPI {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Printf("Fail decoding JSON body: %v", err)
			response := struct {
				Error string `json:"error"`
			}{Error: "Invalid JSON body"}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
		url = body.URL
	} else {
		w.Header().Set("Content-Type", "text/plain")
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Fail reading body: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		url = string(bodyBytes)
	}
	if len(url) == 0 || !strings.HasPrefix(url, "http") {
		log.Printf("Invalid URL: %v", url)
		if isAPI {
			response := struct {
				Error string `json:"error"`
			}{Error: "Invalid URL"}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
		} else {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
		}
		return
	}
	urlModel := model.URLModel{
		URL: url,
	}
	savedModel, err := h.repo.SaveURL(urlModel)
	if err != nil {
		log.Printf("fail to save URL: %v", err)
		if strings.Contains(err.Error(), "failed to generate unique short ID after 5 attempts") {
			if isAPI {
				response := struct {
					Error string `json:"error"`
				}{Error: "Server overloaded, try again later"}
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(response)
			} else {
				http.Error(w, "Server overloaded, try again later", http.StatusServiceUnavailable)
			}
		} else {
			if isAPI {
				response := struct {
					Error string `json:"error"`
				}{Error: http.StatusText(http.StatusInternalServerError)}
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(response)
			} else {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}
		return
	}
	shortURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.cfg.BaseURL, "/"), savedModel.ID)
	w.WriteHeader(http.StatusCreated)
	if isAPI {
		response := struct {
			Result string `json:"result"`
		}{Result: shortURL}
		json.NewEncoder(w).Encode(response)
	} else {
		w.Write([]byte(shortURL))
	}
}
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("Method not allowed: %s", r.Method)
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
	w.Header().Set("Location", urlModel.URL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}
