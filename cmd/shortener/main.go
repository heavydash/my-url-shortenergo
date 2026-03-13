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
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // Подключает pprof для профилирования
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	_ "github.com/joho/godotenv/autoload" // Автоматическая загрузка .env файла
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/deleter"
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

	// Создаём deleter
	urlDeleter := deleter.NewURLDeleter(
		repo,
		logger,
		cfg.DeletionQueueBuffer,
		cfg.DeletionFlushInterval,
		cfg.DeletionMaxBatchSize,
	)
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

	// Запуск сервера с явным контролем над портом и флагами
	logger.Info("Try to start srv", zap.String("requested_addr", srv.Addr))

	// Создаём низкоуровневый TCP-listener
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	// Доступ к файловому дескриптору сокета
	if tcpLn, ok := ln.(*net.TCPListener); ok {
		file, err := tcpLn.File()
		if err != nil {
			logger.Error("failed to get file descriptor", zap.Error(err))
		} else {
			fd := file.Fd()
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			logger.Info("SO_REUSEADDR and SO_REUSEPORT enabled")
		}
	}

	// Логируем реальный порт, на который привязались
	realAddr := ln.Addr().String()
	logger.Info("Srv conn to port", zap.String("actual_addr", realAddr))

	// Запускаем HTTP-сервер на этом listener
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal("failed to serve", zap.Error(err))
	}

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
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		stop := <-quit

		logger.Info("Received signal", zap.String("signal", stop.String()))

		// Контекст с таймаутом для завершения активных запросов
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		// Останавливаем HTTP-сервер
		logger.Info("HTTP server shutting down...")
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown server error", zap.Error(err))
		} else {
			logger.Info("HTTP-server has been stopped gracefully")
		}

		// Даём время deleter'у завершить все задачи
		logger.Info("Deleter has been stopped...")

		if urlDeleter != nil {
			if err := urlDeleter.Close(); err != nil {
				logger.Error("Error of deleter service", zap.Error(err))
			} else {
				logger.Info("Deleter has been stopped gracefully")
			}
		}

		// Останавливаем аудит-сервис (с отдельным таймаутом, если нужно больше времени)
		auditCtx, auditCancel := context.WithTimeout(context.Background(), cfg.AuditShutdownTimeout)
		defer auditCancel()

		logger.Info("Stop audit service...")
		if err := auditSvc.Shutdown(auditCtx); err != nil {
			logger.Error("Error of graceful shutdown audit service", zap.Error(err))
		} else {
			logger.Info("Audit service has been stopped gracefully")
		}

		// Если используется PostgreSQL, закрываем соединение
		if pgRepo, ok := repo.(*repository.PostgresRepository); ok {
			logger.Info("Close connection to PostgreSQL...")
			if err := pgRepo.Close(); err != nil {

				logger.Error("failed to close postgres repo", zap.Error(err))
			}
		}

		// Безопасно закрываем логгер
		logger.Info("Close logger...")
		if err := zap.L().Sync(); err != nil {
			// Игнори ошибок при быстром закрытии в терминале/тестах
			if !errors.Is(err, syscall.ENOTTY) && // inappropriate ioctl
				!errors.Is(err, syscall.EBADF) && // bad file descriptor
				!strings.Contains(err.Error(), "sync /dev/stderr") {
				logger.Error("Error sync of logger", zap.Error(err))
			}
		}

		logger.Info("Server has been gracefully shutdown")
	}()
	// Запуск сервера
	logger.Info("Server has been started", zap.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("Server has been stopped")
		} else {
			logger.Fatal("Error of starting of the server", zap.Error(err))
		}
	}
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
