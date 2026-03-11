// Пакет main реализует точку входа для сервиса сокращателя URL.
//
// Сервис предоставляет HTTP API для создания и управления короткими ссылками,
// поддерживает различные типы хранилищ (память, файл, PostgreSQL), аудит действий
// и graceful shutdown для корректного завершения работы.
//
// Основные возможности:
//   - Создание коротких ссылок из длинных URL
//   - Редирект по короткой ссылке на оригинальный URL
//   - Пакетная обработка ссылок
//   - Авторизация пользователей через cookie
//   - Аудит действий (запись в файл или отправка на удалённый сервер)
//   - Graceful shutdown с сохранением данных
//   - Поддержка профилирования через pprof
//
// Запуск:
//
//	go run ./cmd/shortener
//
// Конфигурация через переменные окружения или флаги:
//   - SERVER_ADDRESS - адрес HTTP-сервера (по умолчанию :8080)
//   - BASE_URL - базовый URL для сокращённых ссылок (по умолчанию http://localhost:8080)
//   - DATABASE_DSN - строка подключения к PostgreSQL (опционально)
//   - FILE_STORAGE_PATH - путь к файловому хранилищу (опционально)
//   - AUDIT_FILE_PATH - путь к файлу аудита (опционально)
//   - AUDIT_REMOTE_URL - URL для отправки аудита (опционально)
//   - SHUTDOWN_TIMEOUT - таймаут graceful shutdown (по умолчанию 10s)
//   - INIT_TIMEOUT - таймаут инициализации (по умолчанию 5s)
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	_ "github.com/joho/godotenv/autoload" // Автоматическая загрузка .env файла
	"go.uber.org/zap"
	"log"
	"net/http"
	_ "net/http/pprof" // Подключает pprof для профилирования
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/heavydash/my-url-shortenergo/migrations"
)

// Переменные версии заполняются при сборке через ldflags.
// Пример сборки:
//
//	go build -ldflags "-X main.buildVersion=1.0.0 -X main.buildDate=$(date -u +%Y-%m-%d) -X main.buildCommit=$(git rev-parse --short HEAD)" -o shortener ./cmd/shortener
var (
	buildVersion string // Версия сборки
	buildDate    string // Дата сборки
	buildCommit  string // Хэш коммита
)

// main — точка входа в приложение.
//
// Алгоритм работы:
// 1. Вывод информации о сборке
// 2. Загрузка конфигурации из переменных окружения/флагов
// 3. Инициализация логгера
// 4. Запуск миграций БД (если используется PostgreSQL)
// 5. Выбор и инициализация хранилища (приоритет: PostgreSQL > файл > память)
// 6. Настройка системы аудита
// 7. Создание HTTP-обработчиков и middleware
// 8. Настройка роутинга
// 9. Запуск HTTP-сервера и pprof
// 10. Ожидание сигнала завершения
// 11. Graceful shutdown с закрытием всех ресурсов
func main() {

	// Вывод информации о сборке
	// Если переменные не заданы, выводится "N/A"
	fmt.Printf("Build version: %s\n", valueOrNA(buildVersion))
	fmt.Printf("Build date:    %s\n", valueOrNA(buildDate))
	fmt.Printf("Build commit:  %s\n", valueOrNA(buildCommit))
	fmt.Println("---")

	// Инициализация конфигурации
	// Читает переменные окружения и флаги командной строки
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Создание логгера (JSON-формат, уровни info и выше)
	logger, _ := zap.NewProduction()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	// Запуск миграций базы данных, если указан DSN
	// Миграции выполняются перед инициализацией репозитория
	if cfg.DatabaseDSN != "" {
		logger.Info("running database migrations...")
		if err := migrations.RunMigrations(cfg.DatabaseDSN); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migrations completed")
	}

	// Выбор хранилища данных
	// Приоритет:
	// 1. PostgreSQL (если указан DatabaseDSN и подключение успешно)
	// 2. Файловое хранилище (если указан FileStoragePath)
	// 3. In-memory хранилище (по умолчанию)
	var repo repository.URLRepository

	if cfg.DatabaseDSN != "" {
		// Пытаемся подключиться к PostgreSQL
		ctx, cancel := context.WithTimeout(context.Background(), cfg.InitTimeout)
		defer cancel()

		pool, err := db.New(ctx, cfg.DatabaseDSN, cfg)
		if err != nil {
			// Если PostgreSQL недоступен, падаем обратно на файл/память
			logger.Warn("postgres unavailable, falling back to file/memory", zap.Error(err))
			repo = repository.NewMemoryRepository(cfg.BaseURL)
		} else {
			logger.Info("using postgres storage")
			repo = repository.NewPostgresRepository(pool.Pool, logger, cfg.BaseURL)
		}
	} else if cfg.FileStoragePath != "" {
		// Используем файловое хранилище
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		repo = repository.NewFileRepository(cfg.FileStoragePath, cfg.BaseURL)
	} else {
		// Используем in-memory хранилище
		logger.Info("using in-memory storage")
		repo = repository.NewMemoryRepository(cfg.BaseURL)
	}

	// Настройка системы аудита
	// Аудит может отправляться в несколько мест одновременно
	var auditSenders []sender.Sender

	if cfg.AuditFilePath != "" {
		// Отправка аудита в файл
		auditSenders = append(auditSenders,
			sender.NewFileSender(cfg.AuditFilePath, logger))

	}

	if cfg.AuditRemoteURL != "" {
		// Отправка аудита на удалённый HTTP-сервер
		auditSenders = append(auditSenders,
			sender.NewHTTPSender(cfg.AuditRemoteURL, cfg.HTTPClientTimeout, logger))
	}

	// Создание сервиса аудита с несколькими отправителями
	auditSvc := service.NewAuditService(cfg, logger, auditSenders...)

	// Создание основного обработчика HTTP-запросов
	h := handler.NewHandler(repo, cfg, logger, auditSvc)

	// Настройка роутера Chi
	router := chi.NewRouter()

	// Глобальные middleware для всех запросов
	router.Use(middleware.Logging(logger))        // Логирование запросов
	router.Use(middleware.GzipMiddleware(logger)) // Сжатие ответов

	// Публичный маршрут для редиректа по короткой ссылке
	router.Get("/{id}", h.RedirectURL)

	// Кастомная обработка 404
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Группа авторизованных маршрутов
	// Требуют валидной cookie-авторизации
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(logger))
		r.Get("/api/user/urls", h.GetUserURLs)   // Получение всех ссылок пользователя
		r.Delete("/api/user/urls", h.DeleteUrls) // Удаление ссылок пользователя
	})

	// Публичные маршруты
	router.Get("/ping", h.PingHandler) // Проверка доступности сервера
	router.Get("/", h.HomeHandler)     // Домашняя страница

	// Маршруты с опциональной авторизацией
	// Если cookie есть — используем её, если нет — создаём новую
	router.Post("/", middleware.Auth(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, false) // Создание короткой ссылки (обычный запрос)
	})).ServeHTTP)

	router.Post("/api/shorten", middleware.Auth(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ShortenHandler(w, r, true) // Создание короткой ссылки (JSON API)
	})).ServeHTTP)

	router.Post("/api/shorten/batch", middleware.Auth(logger)(http.HandlerFunc(h.BatchShortenHandler)).ServeHTTP) // Пакетное создание

	// Настройка HTTP-сервера
	srv := &http.Server{
		Addr:    cfg.ServerAddr, // Адрес для прослушивания
		Handler: router}

	// Запуск сервера в горутине
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	logger.Info("server started", zap.String("addr", cfg.ServerAddr))

	// Запуск pprof для профилирования
	// Доступно по адресу http://localhost:6060/debug/pprof/
	go func() {
		pprofAddr := "localhost:6060"
		logger.Info("pprof server started", zap.String("addr", "http://"+pprofAddr+"/debug/pprof"))

		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("failed to start pprof server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	// Ожидаем сигналы SIGINT (Ctrl+C) или SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down server...")

	// Контекст с таймаутом для завершения активных запросов
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Останавливаем HTTP-сервер
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
	}

	// Закрываем удалятор ссылок (если есть активные задачи)
	if err := h.Close(); err != nil {
		logger.Error("deleter shutting down failed", zap.Error(err))
	}

	// Отдельный контекст для завершения аудита
	// Аудиту даётся больше времени на отправку всех событий
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.AuditShutdownTimeout)
	defer cancel()

	if err := auditSvc.Shutdown(shutdownCtx); err != nil {
		logger.Error("audit shutdown failed", zap.Error(err))
	} else {
		logger.Info("audit shutdown gracefully")
	}

	// Если используется PostgreSQL, закрываем соединение
	if pgRepo, ok := repo.(*repository.PostgresRepository); ok {
		if err := pgRepo.Close(); err != nil {
			logger.Error("failed to close postgres repo", zap.Error(err))
		}
	}
	logger.Info("server stopped gracefully")

}

// valueOrNA возвращает переданную строку или "N/A", если строка пустая.
//
// Используется для форматированного вывода информации о сборке.
// Если переменные версии не были заданы при сборке, выводится "N/A".
//
// Параметры:
//   - s: строка для проверки
//
// Возвращает:
//   - исходную строку, если она не пустая
//   - "N/A", если строка пустая
func valueOrNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}
