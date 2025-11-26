package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/pressly/goose/v3"
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
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Config loaded", zap.String("BaseURL", cfg.BaseURL))

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrations(cfg.DatabaseDSN, logger); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migrations applied successfully")
		os.Exit(0)
	}

	repo := repository.NewFactory(cfg, logger)
	h := handler.NewHandler(repo, cfg, logger)

	r := chi.NewRouter()
	r.Use(middleware.GzipMiddleware)
	r.Use(middleware.Logging(logger))
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

func runMigrations(dsn string, logger *zap.Logger) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		return err
	}

	return goose.Up(db, "migrations")
}
