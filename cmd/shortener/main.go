package main

import (
	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"log"
	"net/http"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	repo := repository.NewMemoryRepository()
	h := handler.NewHandler(repo, cfg)
	r := chi.NewRouter()
	h.SetupRoutes(r)
	log.Printf("Starting server on %s with BaseURL %s", cfg.ServerAddr, cfg.BaseURL)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
