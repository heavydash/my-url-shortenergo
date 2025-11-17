package main

import (
	"context"
	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"go.uber.org/zap"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("logger sync error: %v", err)
		}
	}()

	repo := repository.New(cfg, logger)

	h := handler.NewHandler(repo, cfg, logger)

	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)

	srv := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: r}

	go func() {
		logger.Info("starting server", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	logger.Info("shutting down server")

	if err := srv.Shutdown(context.Background()); err != nil {
		logger.Fatal("failed to shutdown server", zap.Error(err))
	} else {
		logger.Info("server stopped")
	}

	if pgRepo, ok := repo.(*repository.PostgresRepository); ok {
		pgRepo.Pool.Close()
		logger.Info("postgres pool closed")
	}

}
