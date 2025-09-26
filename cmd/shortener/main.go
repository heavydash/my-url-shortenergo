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
	cfg := config.NewConfig()
	repo := repository.NewMemoryRepository()
	h := handler.NewHandler(repo, cfg)
	r := chi.NewRouter()
	h.SetupRoutes(r)
	log.Fatal(http.ListenAndServe(cfg.ServerAddr, r))
}
