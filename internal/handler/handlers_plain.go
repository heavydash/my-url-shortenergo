package handler

import (
	"go.uber.org/zap"
	"log"
	"net/http"
	"strings"
)

func (h *Handler) ShortenPlainHandler(w http.ResponseWriter, r *http.Request) {
	h.ShortenHandler(w, r, false)
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
	h.logger.Info("Redirect handler called", zap.String("path", r.URL.Path))

	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
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
