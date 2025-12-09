package main

import (
	"context"
	"errors"
	_ "github.com/joho/godotenv/autoload"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/migrations"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	if cfg.DatabaseDSN != "" {
		logger.Info("running database migrations...")
		if err := migrations.RunMigrations(cfg.DatabaseDSN); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migrations completed")
	}

	repo := repository.NewFactory(cfg, logger)
	h := handler.NewHandler(repo, cfg, logger)

	router := chi.NewRouter()

	// Глобальные
	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware)

	router.Get("/{id}", h.RedirectURL)

	// Авторизованные роуты
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth())
		r.Get("/api/user/urls", h.GetUserURLs)
	})

	// Анонимные
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	router.Post("/", h.ShortenPlainHandler)
	router.Post("/api/shorten", h.ShortenJSONHandler)
	router.Post("/api/shorten/batch", h.BatchShortenHandler)

	srv := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: router}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	logger.Info("server started", zap.String("addr", cfg.ServerAddr))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	logger.Info("server stopped")
}
