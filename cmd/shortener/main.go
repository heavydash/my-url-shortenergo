package main

import (
	"context"
	"errors"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
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
	// Конфиг
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}
	// Логгер
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Миграции, если есть DSN
	if cfg.DatabaseDSN != "" {
		logger.Info("running database migrations...")
		if err := migrations.RunMigrations(cfg.DatabaseDSN); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migrations completed")
	}

	// Создаем репозиторий
	var repo repository.URLRepository

	if cfg.DatabaseDSN != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pool, err := db.New(ctx, cfg.DatabaseDSN)
		if err != nil {
			logger.Warn("postgres unavailable, falling back to file/memory", zap.Error(err))
			repo = repository.NewMemoryRepository()
		} else {
			logger.Info("using postgres storage")
			repo = repository.NewPostgresRepository(pool.Pool, logger)
		}
	} else if cfg.FileStoragePath != "" {
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		repo = repository.NewFileRepository(cfg.FileStoragePath)
	} else {
		logger.Info("using in-memory storage")
		repo = repository.NewMemoryRepository()
	}

	// Хендлер
	h := handler.NewHandler(repo, cfg, logger)

	// Роутер
	router := chi.NewRouter()

	// Глобальные
	router.Use(middleware.Logging(logger))
	router.Use(middleware.GzipMiddleware)

	router.Get("/{id}", h.RedirectURL)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Авторизованные роуты
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))
		r.Get("/api/user/urls", h.GetUserURLs)
		r.Delete("/api/user/urls", h.DeleteUrls)
	})

	// Анонимные
	router.Get("/ping", h.PingHandler)
	router.Get("/", h.HomeHandler)

	router.Post("/", h.ShortenPlainHandler)
	router.Post("/api/shorten", h.ShortenJSONHandler)
	router.Post("/api/shorten/batch", h.BatchShortenHandler)

	// Сервер
	srv := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: router}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	logger.Info("server started", zap.String("addr", cfg.ServerAddr))

	// Gracefull shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	logger.Info("server stopped")
}
