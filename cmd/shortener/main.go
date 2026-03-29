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
	"github.com/go-chi/chi/v5"
	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"github.com/heavydash/my-url-shortenergo/internal/audit/service"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"github.com/heavydash/my-url-shortenergo/internal/deleter"
	"github.com/heavydash/my-url-shortenergo/internal/handler"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/migrations"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // Подключает pprof для профилирования
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/joho/godotenv/autoload" // Автоматическая загрузка .env файла
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/heavydash/my-url-shortenergo/internal/repository"
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

	// Создание логгера (JSON-формат, уровни info и выше)
	logger, _ := zap.NewProduction()
	defer func() {
		if err := logger.Sync(); err != nil {
			panic(err)
		}
	}()

	// Инициализация конфигурации
	// Читает переменные окружения и флаги командной строки
	cfg, err := config.NewConfig(logger)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}
	logger.Debug("Config after load:\n%+v")

	if err := cfg.Validate(logger); err != nil {
		logger.Fatal("invalid configuration", zap.Error(err))
	}

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
			repo = repository.NewMemoryRepository(cfg.BaseURL, logger)
		} else {
			logger.Info("using postgres storage")
			repo = repository.NewPostgresRepository(pool.Pool, logger, cfg.BaseURL)
		}
	} else if cfg.FileStoragePath != "" {
		// Используем файловое хранилище
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		repo = repository.NewFileRepository(cfg.FileStoragePath, cfg.BaseURL, logger)
	} else {
		// Используем in-memory хранилище
		logger.Info("using in-memory storage")
		repo = repository.NewMemoryRepository(cfg.BaseURL, logger)
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

	// Маршруты с опциональной авторизацией
	// Внутренние служебные эндпоинты (для мониторинга, администрирования и т.д.)
	// Защищены проверкой IP-подсети (trusted_subnet), а не cookie
	router.Group(func(r chi.Router) {
		r.Get("/api/internal/stats", h.GetInternalStats)
	})

	// Настройка graceful shutdown с отслеживанием сигналов ОС.
	// Поддерживаются сигналы:
	//   - Ctrl+C (SIGINT)
	//   - SIGTERM (стандартный сигнал завершения от ОС)
	//   - SIGQUIT (обычно генерируется при завершении терминала)
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer stop() // Освобождаем ресурсы signal.NotifyContext после завершения

	// Настройка HTTP-сервера с таймаутами.
	// ReadTimeout  - таймаут на чтение всего запроса (включая тело)
	// WriteTimeout - таймаут на отправку ответа
	// IdleTimeout  - таймаут на keep-alive соединения
	srv := &http.Server{
		Addr:    cfg.ServerAddr, // Адрес для прослушивания
		Handler: router,

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  65 * time.Second,
	}

	// Запуск сервера с явным контролем над портом и флагами
	logger.Info("Try to start srv", zap.String("requested_addr", srv.Addr))

	// Создаём TCP-listener вручную, чтобы иметь доступ к сокету.
	// Это даёт возможность установить SO_REUSEADDR и SO_REUSEPORT
	// для быстрого перезапуска без ошибки "address already in use".
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	// Включаем опции сокета для быстрого переиспользования порта.
	// SO_REUSEADDR - позволяет переиспользовать порт в состоянии TIME_WAIT
	// SO_REUSEPORT - позволяет нескольким процессам слушать один порт
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

	// Логируем реальный порт, на который привязались.
	// Может отличаться от запрошенного, если указан ":0" (случайный порт).
	realAddr := ln.Addr().String()
	logger.Info("Srv conn to port", zap.String("actual_addr", realAddr))

	// Канал для ошибок сервера, чтобы не блокировать main горутину.
	serverErr := make(chan error, 1)

	// Запускаем HTTP-сервер в отдельной горутине.
	go func() {
		logger.Info("Starting HTTP server")
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		log.Println("HTTP server stopped")
	}()

	logger.Info("server started", zap.String("addr", cfg.ServerAddr))

	// Запуск pprof сервера для профилирования в отдельной горутине.
	// Доступен по адресу http://localhost:6060/debug/pprof/
	go func() {
		pprofAddr := "localhost:6060"
		logger.Info("pprof server started", zap.String("addr", "http://"+pprofAddr+"/debug/pprof"))

		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("failed to start pprof server", zap.Error(err))
		}
	}()

	startTime := time.Now() // Засекаем время старта для логов

	// Ожидаем либо ошибку сервера, либо сигнал завершения.
	select {
	case err := <-serverErr:
		// Сервер упал сам по себе (не по сигналу)
		logger.Fatal("HTTP server failed to start", zap.Error(err))

	case <-ctx.Done():
		// Получен сигнал завершения (Ctrl+C, SIGTERM, SIGQUIT)
		logger.Info("Shutdown signal received",
			zap.String("signal", ctx.Err().Error()),
			zap.Duration("time_since_start", time.Since(startTime)))

		stop() // второй Ctrl+C теперь мгновенно убивает

		// Graceful shutdown HTTP сервера.
		// Даём серверу cfg.ShutdownTimeout секунд на завершение активных запросов.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		logger.Info("Shutting down HTTP server...")

		// Запускаем Shutdown в фоне, чтобы не блокировать остальные компоненты.
		go func() {
			_ = srv.Shutdown(shutdownCtx)
		}()

		<-shutdownCtx.Done() // Ждём завершения или таймаута

		// Закрываем URLDeleter (асинхронный удалятор).
		// Deleter имеет свой внутренний буфер, нужно дать ему время дописать.
		deleterStart := time.Now()
		logger.Info("Closing URL deleter...")
		if urlDeleter != nil {
			if err := urlDeleter.Close(); err != nil {
				logger.Error("failed to close url deleter", zap.Error(err))
			} else {
				logger.Info("url deleter closed")
			}
		}
		logger.Info("URL deleter closed", zap.Duration("took", time.Since(deleterStart)))

		// Закрываем сервис аудита.
		// У него свой буфер событий, нужно дождаться отправки всех.
		auditStart := time.Now()
		logger.Info("Shutting down audit service...")
		auditCtx, auditCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer auditCancel()
		if err := auditSvc.Shutdown(auditCtx); err != nil {
			logger.Error("Audit shutdown failed", zap.Error(err))
		} else {
			logger.Info("Audit shutdown completed", zap.Duration("took", time.Since(auditStart)))
		}

		// Закрываем соединение с PostgreSQL, если оно было открыто.
		if pgRepo, ok := repo.(*repository.PostgresRepository); ok {
			pgStart := time.Now()
			logger.Info("Closing PostgreSQL...")
			if err := pgRepo.Close(); err != nil {
				logger.Error("failed to close postgres repository", zap.Error(err))
			} else {
				logger.Info("PostgreSQL closed", zap.Duration("took", time.Since(pgStart)))
			}
		}
		logger.Info("All resources finished gracefully")
		logger.Sync() // Сбрасываем буфер логгера

		// Форс-мажорный выход через 2 секунды, если что-то зависло.
		go func() {
			time.Sleep(2 * time.Second)
			logger.Fatal("Принудительный выход — что-то зависло")
			os.Exit(1)
		}()

		os.Exit(0) // Нормальный выход
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
