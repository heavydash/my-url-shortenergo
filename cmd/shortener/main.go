package main

import (
	"github.com/go-chi/chi"
	"log"
	"net/http"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}
	repo := repository.NewMemoryRepository()
	h := handler.NewHandler(repo, cfg)
	r := chi.NewRouter()
	h.SetupRoutes(r)
	log.Printf("Starting server on %S", cfg.ServerAddr)
	log.Fatal(http.ListenAndServe(cfg.ServerAddr, r))
}
