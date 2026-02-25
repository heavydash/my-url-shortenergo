package main

import (
	"context"
	"errors"
	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/migrations"
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
		ctx, cancel := context.WithTimeout(context.Background(), cfg.InitTimeout)
		defer cancel()

		pool, err := db.New(ctx, cfg.DatabaseDSN, cfg)
		if err != nil {
			logger.Warn("postgres unavailable, falling back to file/memory", zap.Error(err))
			repo = repository.NewMemoryRepository(cfg.BaseURL)
		} else {
			logger.Info("using postgres storage")
			repo = repository.NewPostgresRepository(pool.Pool, logger, cfg.BaseURL)
		}
	} else if cfg.FileStoragePath != "" {
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		repo = repository.NewFileRepository(cfg.FileStoragePath, cfg.BaseURL)
	} else {
		logger.Info("using in-memory storage")
		repo = repository.NewMemoryRepository(cfg.BaseURL)
	}

	// Аудит
	var auditSenders []sender.Sender

	if cfg.AuditFilePath != "" {
		auditSenders = append(auditSenders,
			sender.NewFileSender(cfg.AuditFilePath, logger))

	}

	if cfg.AuditRemoteURL != "" {
		auditSenders = append(auditSenders,
			sender.NewHTTPSender(cfg.AuditRemoteURL, cfg.HTTPClientTimeout, logger))
	}

	auditSvc := service.NewAuditService(cfg, logger, auditSenders...)

	// Хендлер
	h := handler.NewHandler(repo, cfg, logger, auditSvc)

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

	router.Post("/", middleware.Auth(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, false)
	})).ServeHTTP)

	router.Post("/api/shorten", middleware.Auth(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, true)
	})).ServeHTTP)

	router.Post("/api/shorten/batch", middleware.Auth(logger)(http.HandlerFunc(h.BatchShortenHandler)).ServeHTTP)
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

	// Профилирование
	go func() {
		pprofAddr := "localhost:6060"
		logger.Info("pprof server started", zap.String("addr", "http://"+pprofAddr+"/debug/pprof"))

		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("failed to start pprof server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
	}

	if err := h.Close(); err != nil {
		logger.Error("deleter shutting down failed", zap.Error(err))
	}

	// Даем дописать и успешно shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.AuditShutdownTimeout)
	defer cancel()

	if err := auditSvc.Shutdown(shutdownCtx); err != nil {
		logger.Error("audit shutdown failed", zap.Error(err))
	} else {
		logger.Info("audit shutdown gracefully")
	}

	if pgRepo, ok := repo.(*repository.PostgresRepository); ok {
		if err := pgRepo.Close(); err != nil {
			logger.Error("failed to close postgres repo", zap.Error(err))
		}
	}
	logger.Info("server stopped gracefully")

}
