package main

import (
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"log"
	"net/http"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	repo, err := repository.NewFileRepository(cfg.FileStorage)
	if err != nil {
		logger.Fatal("failed to create repository", zap.Error(err))
	}
	h := handler.NewHandler(repo, cfg)
	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		logger.Fatal("Fail to server start", zap.Error(err))
	}
}
