package main

import (
	"context"
	"errors"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/migrations"
	"go.uber.org/zap"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, continuing without it")
	}

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

	r := chi.NewRouter()

	r.Use(middleware.Auth())

	r.Use(middleware.Logging(logger))
	r.Use(middleware.GzipMiddleware)
	h.SetupRoutes(r)

	srv := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: r}

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
