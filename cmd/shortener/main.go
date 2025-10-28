package main

import (
	"flag"
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

	flag.String("a", "", "server address")
	flag.String("b", "", "base URL")
	flag.Parse()

	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}
	fileCfg := config.NewFileStorageConfig()

	var repo repository.URLRepository
	if fileCfg.Path != "" {
		repo = repository.NewFileRepository(fileCfg.Path)
	} else {
		repo = repository.NewMemoryRepository()
	}
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()
	h := handler.NewHandler(repo, cfg)
	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)
	if err := http.ListenAndServe(cfg.ServerAddr, r); err != nil {
		logger.Fatal("Fail to server start", zap.Error(err))
	}
}
