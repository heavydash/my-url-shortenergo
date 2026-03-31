// Package config предоставляет загрузку и управление конфигурацией приложения.
// Поддерживает multiple источников конфигурации: флаги командной строки и переменные окружения
// с приоритетом переменных окружения (env overrides flags).
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"go.uber.org/zap"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// DBConfig содержит настройки пула соединений с PostgreSQL.
type DBConfig struct {
	DBMaxConns          int           `json:"db_max_conns"`
	DBMinConns          int           `json:"db_min_conns"`
	DBMaxConnLifetime   time.Duration `json:"db_max_conn_lifetime"`
	DBHealthCheckPeriod time.Duration `json:"db_health_check_period"`
}

// Config представляет полную конфигурацию URL shortener сервиса.
//
// Конфигурация загружается в следующем порядке приоритета:
//  1. Значения по умолчанию (hardcoded в флагах)
//  2. Флаги командной строки (переопределяют defaults)
//  3. Переменные окружения (переопределяют flags, высший приоритет)
//
// Пример использования флагов:
//
//	./app -a :8080 -b http://localhost:8080 -f /tmp/urls.json
//
// Пример использования env переменных:
//
//	SERVER_ADDRESS=:9090 BASE_URL=https://short.ly ./app
type Config struct {

	// ServerAddr - адрес и порт для запуска HTTP сервера.
	// Формат: ":8080", "localhost:8080", "0.0.0.0:8080"
	// Флаг: -a, env: SERVER_ADDRESS
	ServerAddr string

	// BaseURL - базовый URL для сокращенных ссылок.
	// Используется для формирования полного сокращенного URL.
	// Пример: "http://localhost:8080", "https://short.ly"
	// Флаг: -b, env: BASE_URL
	BaseURL string

	// FileStoragePath - путь к файлу для хранения URL (если используется file storage).
	// Если пустой - file storage не используется.
	// Флаг: -f, env: FILE_STORAGE_PATH
	FileStoragePath string

	// DatabaseDSN - Data Source Name для подключения к PostgreSQL.
	// Формат: "postgres://user:pass@host:port/dbname?sslmode=disable"
	// Если пустой - база данных не используется.
	// Флаг: -d, env: DATABASE_DSN
	DatabaseDSN string

	// Поля для Deleter'а (асинхронного удаления URL)

	// DeletionQueueBuffer - размер буфера очереди задач на удаление.
	// Определяет сколько задач на удаление могут быть в буфере до блокировки.
	// Флаг: -dq, env: DELETION_QUEUE_BUFFER
	DeletionQueueBuffer int

	// DeletionFlushInterval - интервал сброса задач удаления в БД.
	// Меньшие значения уменьшают задержку, но увеличивают нагрузку на БД.
	// Флаг: -df, env: DELETION_FLUSH_INTERVAL
	DeletionFlushInterval time.Duration

	// DeletionMaxBatchSize - максимальный размер батча для удаления на пользователя.
	// Задачи удаления группируются по пользователям и отправляются батчами.
	// Флаг: -dm, env: DELETION_MAX_BATCH_SIZE
	DeletionMaxBatchSize int

	// Поля для Сервиса Аудита

	// AuditFilePath - путь к файлу для записи аудит-логов.
	// Если пустой - файловый аудит отключен.
	// Флаг: -audit-file, env: AUDIT_FILE
	AuditFilePath string

	// AuditRemoteURL - URL удаленного сервера для отправки аудит-событий.
	// Если пустой - HTTP аудит отключен.
	// Флаг: -audit-url, env: AUDIT_URL
	AuditRemoteURL string

	// Поля для таймаутов

	// ServerTimeout - таймаут обработки HTTP запроса.
	// Используется для ReadTimeout, WriteTimeout и IdleTimeout сервера.
	// Флаг: -server-timeout, env: SERVER_TIMEOUT
	ServerTimeout time.Duration

	// InitTimeout - таймаут инициализации компонентов при старте.
	// Используется для подключения к БД и других инициализаций.
	// Флаг: -init-timeout, env: INIT_TIMEOUT
	InitTimeout time.Duration

	// ShutdownTimeout - таймаут graceful shutdown HTTP сервера.
	// Время на завершение активных запросов перед остановкой.
	// Флаг: -shutdown-timeout, env: SHUTDOWN_TIMEOUT
	ShutdownTimeout time.Duration

	// PingTimeout - таймаут проверки соединения с БД.
	// Используется для Ping операций.
	// Флаг: -ping-timeout, env: PING_TIMEOUT
	PingTimeout time.Duration

	// HTTPClientTimeout - таймаут HTTP клиента для внешних запросов.
	// Используется в HTTP отправителе аудита.
	// Флаг: -http-client-timeout, env: HTTP_CLIENT_TIMEOUT
	HTTPClientTimeout time.Duration

	// AuditBufferSize - размер буфера канала событий аудита
	AuditBufferSize int

	// AuditShutdownTimeout — таймаут для graceful завершения сервиса аудита
	// (время, за которое должны быть отправлены / записаны все накопленные события)
	// Обычно чуть больше, чем обычный ShutdownTimeout
	AuditShutdownTimeout time.Duration

	// DB содержит все настройки пула соединений с PostgreSQL
	DB DBConfig `json:"db"`

	// Поля для HTTPS

	// EnableHTTPS - включение HTTPS (TLS) для сервера.
	// Флаг: -enable-https, env: ENABLE_HTTPS
	EnableHTTPS bool

	// TLSCert - путь к файлу сертификата TLS.
	// Флаг: -tls-cert, env: TLS_CERT
	TLSCert string

	// TLSKey - путь к файлу приватного ключа TLS.
	// Флаг: -tls-key, env: TLS_KEY
	TLSKey string

	// TrustedSubnet — CIDR-нотация доверенной подсети, из которой разрешён доступ
	// к внутренним административным эндпоинтам (например, /api/internal/stats).
	//
	// Примеры корректных значений:
	//   "127.0.0.1/32"     — только localhost
	//   "192.168.0.0/16"   — вся локальная сеть 192.168.0.0–192.168.255.255
	//   "10.0.0.0/8"       — вся подсеть 10.0.0.0/8
	//   "172.16.0.0/12"    — подсеть Docker/Kubernetes по умолчанию
	//
	// Если поле пустое (""), то:
	//   • доступ к /api/internal/stats будет запрещён для всех IP-адресов
	//   • isRequestFromTrustedSubnet() всегда возвращает false
	//   • в логах при старте будет сообщение "trusted subnet is empty - stats will be forbidden for everyone"
	//
	// Проверка выполняется по заголовку X-Real-IP (не по r.RemoteAddr).
	//
	// Флаг: -t, env: TRUSTED_SUBNET
	// JSON: "trusted_subnet"
	TrustedSubnet string `json:"trusted_subnet"`

	// TrustedSubnetNet — разобранная CIDR-подсеть.
	// Заполняется автоматически в NewConfig() после парсинга TrustedSubnet.
	// Не экспортируется в JSON и не должна устанавливаться вручную.
	// Если TrustedSubnet пустой или некорректный — остаётся nil.
	TrustedSubnetNet *net.IPNet `json:"-"`

	// logger - внутренний логгер для конфигурации.
	// Не экспортируется, используется для отладки загрузки.
	logger *zap.Logger
}

// NewConfig создает и загружает конфигурацию приложения.
//
// Процесс загрузки:
//  1. Парсинг флагов командной строки
//  2. Установка значений из флагов (или defaults)
//  3. Перезапись значений из переменных окружения (если установлены)
//
// Возвращает:
//   - *Config: загруженную конфигурацию
//   - error: ошибка парсинга флагов командной строки
//
// Пример использования:
//
//	cfg, err := config.NewConfig()
//	if err != nil {
//	    log.Fatal("Failed to load config:", err)
//	}
//
// Доступные флаги командной строки:
//
//	-a    : адрес сервера (default: ":8080")
//	-b    : базовый URL (default: "http://localhost:8080")
//	-f    : путь к файлу хранилища
//	-d    : DSN для PostgreSQL
//	-dq   : размер буфера очереди удаления (default: 1000)
//	-df   : интервал сброса удаления (default: "50ms")
//	-dm   : максимальный размер батча удаления (default: 1000)
//	-audit-file : путь к файлу аудита
//	-audit-url  : URL сервера аудита
//	-audit-buffer : размер буфера аудита (default: 4096)
//	-audit-shutdown-timeout : таймаут shutdown аудита (default: 15s)
//	-db-max-conns : максимум соединений с БД (default: 20)
//	-db-min-conns : минимум соединений с БД (default: 5)
//	-db-max-lifetime : макс. время жизни соединения (default: 5m)
//	-db-health-period : период проверки здоровья (default: 1m)
//	-server-timeout : таймаут HTTP сервера (default: 5s)
//	-init-timeout : таймаут инициализации (default: 15s)
//	-shutdown-timeout : таймаут graceful shutdown (default: 10s)
//	-ping-timeout : таймаут ping БД (default: 2s)
//	-http-client-timeout : таймаут HTTP клиента (default: 5s)
//	-enable-https : включение HTTPS (default: false)
//	-tls-cert : путь к TLS сертификату (default: "server.crt")
//	-tls-key : путь к TLS ключу (default: "server.key")
func NewConfig(logger *zap.Logger) (*Config, error) {
	fs := flag.NewFlagSet("url-shortener", flag.ContinueOnError)

	// Флаг для пути к json
	configFile := fs.String("c", "", "path to JSON config file")

	// Флаги серверные
	a := fs.String("a", ":8080", "address to run HTTP server")
	b := fs.String("b", "http://localhost:8080", "base URL for shortened links")
	f := fs.String("f", "", "file path to store the URL")
	d := fs.String("d", "", "DSN to store the URL")

	// Флаги для Deleter
	dq := fs.Int("dq", 1000, "deletion queue buffer size (default 1000)")
	df := fs.String("df", "50ms", "deletion flush interval, e.g. 50ms, 1s (default 50ms)")
	dm := fs.Int("dm", 1000, "deletion max batch size per user (default 1000)")

	// Флаги для Audit
	auditFileFlag := fs.String("audit-file", "", "path to audit log file")
	auditURLFlag := fs.String("audit-url", "", "remote audit server URL")
	auditBufferSize := fs.Int("audit-buffer", 4096, "audit events channel buffer size")
	auditShutdownTimeout := fs.Duration("audit-shutdown-timeout", 15*time.Second, "audit service graceful shutdown timeout")

	// Флаги для db
	dbMaxConns := fs.Int("db-max-conns", 20, "max connections in pgx pool")
	dbMinConns := fs.Int("db-min-conns", 5, "min connections in pgx pool")
	dbMaxLifetime := fs.Duration("db-max-lifetime", 5*time.Minute, "max connection lifetime")
	dbHealthPeriod := fs.Duration("db-health-period", 1*time.Minute, "health check period")

	// Флаги для Timeout
	serverTimeoutFlag := fs.Duration("server-timeout", 5*time.Second, "HTTP server request timeout")
	initTimeoutFlag := fs.Duration("init-timeout", 15*time.Second, "initialization timeout")
	shutdownTimeoutFlag := fs.Duration("shutdown-timeout", 10*time.Second, "graceful shutdown timeout")
	pingTimeoutFlag := fs.Duration("ping-timeout", 2*time.Second, "DB ping timeout")
	httpClientTimeout := fs.Duration("http-client-timeout", 5*time.Second, "HTTP client timeout")

	// Флаги для HTTPS
	enableHTTPS := fs.Bool("enable-https", false, "enable HTTPS (TLS)")
	tlsCert := fs.String("tls-cert", "server.crt", "path to TLS certificate file")
	tlsKey := fs.String("tls-key", "server.key", "path to TLS private key file")

	// Флаги для Subnet
	trustedSubnet := fs.String("t", "", "trusted subnet in CIDR notation (e.g. 192.168.0.0/16)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// cfg с Дефолтными значениями
	cfg := &Config{
		// Srv
		ServerAddr:      ":8080",
		BaseURL:         "http://localhost:8080",
		FileStoragePath: "",
		DatabaseDSN:     "",
		// Deleter
		DeletionQueueBuffer:   1000,
		DeletionFlushInterval: 50 * time.Millisecond,
		DeletionMaxBatchSize:  1000,
		// Audit
		AuditFilePath:        "",
		AuditRemoteURL:       "",
		AuditBufferSize:      4096,
		AuditShutdownTimeout: 15 * time.Second,
		//db
		DB: DBConfig{
			DBMaxConns:          20,
			DBMinConns:          5,
			DBMaxConnLifetime:   5 * time.Minute,
			DBHealthCheckPeriod: 1 * time.Minute,
		},
		// Timeout
		ServerTimeout:     5 * time.Second,
		InitTimeout:       15 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		PingTimeout:       2 * time.Second,
		HTTPClientTimeout: 5 * time.Second,
		// HTTPS
		EnableHTTPS: false,
		TLSCert:     "",
		TLSKey:      "",
		// Subnet
		TrustedSubnet:    "",
		TrustedSubnetNet: nil,
	}

	// Определяем путь к JSON
	var jsonPath string
	if *configFile != "" {
		jsonPath = *configFile
	} else if val := os.Getenv("CONFIG"); val != "" {
		jsonPath = val
	}

	if jsonPath != "" {
		logger.Info("attempting to load JSON config", zap.String("path", jsonPath))
		if err := loadFromJSON(jsonPath, cfg, logger); err != nil {
			logger.Warn("failed to load JSON config, falling back to flags/env", zap.Error(err))
		} else {
			logger.Info("successfully loaded JSON config", zap.String("path", jsonPath))
		}
	} else {
		logger.Debug("no JSON config file specified, falling back to flags/env")
	}

	// Дефолты флагов для проверки
	// Srv
	defaultA := ":8080"
	defaultB := "http://localhost:8080"
	defaultF := ""
	defaultD := ""
	// Deletion
	defaultDQ := 1000
	defaultDF := "50ms"
	defaultDM := 1000
	// Audit
	defaultAuditFile := ""
	defaultAuditURL := ""
	defaultAuditBuffer := 4096
	defaultAuditShutdown := 15 * time.Second
	// DB
	defaultDBMaxConns := 20
	defaultDBMinConns := 5
	defaultDBMaxLifetime := 5 * time.Minute
	defaultDBHealthPeriod := 1 * time.Minute
	// Timeout
	defaultServerTimeout := 5 * time.Second
	defaultInitTimeout := 15 * time.Second
	defaultShutdownTimeout := 10 * time.Second
	defaultPingTimeout := 2 * time.Second
	// HTTPS
	defaultHTTPClientTimeout := 5 * time.Second
	defaultEnableHTTPS := false
	defaultTLSCert := "server.crt"
	defaultTLSKey := "server.key"

	// Применяем флаги, если они отличаются от дефолта
	if *a != defaultA {
		cfg.ServerAddr = *a
	}
	if *b != defaultB {
		cfg.BaseURL = *b
	}
	if *f != defaultF {
		cfg.FileStoragePath = *f
	}
	if *d != defaultD {
		cfg.DatabaseDSN = *d
	}
	if *dq != defaultDQ {
		cfg.DeletionQueueBuffer = *dq
	}
	if *df != defaultDF {
		cfg.DeletionFlushInterval = parseDuration(*df, 50*time.Millisecond)
	}
	if *dm != defaultDM {
		cfg.DeletionMaxBatchSize = *dm
	}
	if *auditFileFlag != defaultAuditFile {
		cfg.AuditFilePath = *auditFileFlag
	}
	if *auditURLFlag != defaultAuditURL {
		cfg.AuditRemoteURL = *auditURLFlag
	}
	if *auditBufferSize != defaultAuditBuffer {
		cfg.AuditBufferSize = *auditBufferSize
	}
	if *auditShutdownTimeout != defaultAuditShutdown {
		cfg.AuditShutdownTimeout = *auditShutdownTimeout
	}
	if *dbMaxConns != defaultDBMaxConns {
		cfg.DB.DBMaxConns = *dbMaxConns
	}
	if *dbMinConns != defaultDBMinConns {
		cfg.DB.DBMinConns = *dbMinConns
	}
	if *dbMaxLifetime != defaultDBMaxLifetime {
		cfg.DB.DBMaxConnLifetime = *dbMaxLifetime
	}
	if *dbHealthPeriod != defaultDBHealthPeriod {
		cfg.DB.DBHealthCheckPeriod = *dbHealthPeriod
	}
	if *serverTimeoutFlag != defaultServerTimeout {
		cfg.ServerTimeout = *serverTimeoutFlag
	}
	if *initTimeoutFlag != defaultInitTimeout {
		cfg.InitTimeout = *initTimeoutFlag
	}
	if *shutdownTimeoutFlag != defaultShutdownTimeout {
		cfg.ShutdownTimeout = *shutdownTimeoutFlag
	}
	if *pingTimeoutFlag != defaultPingTimeout {
		cfg.PingTimeout = *pingTimeoutFlag
	}
	if *httpClientTimeout != defaultHTTPClientTimeout {
		cfg.HTTPClientTimeout = *httpClientTimeout
	}
	if *enableHTTPS != defaultEnableHTTPS {
		cfg.EnableHTTPS = *enableHTTPS
	}
	if *tlsCert != defaultTLSCert {
		cfg.TLSCert = *tlsCert
	}
	if *tlsKey != defaultTLSKey {
		cfg.TLSKey = *tlsKey
	}
	if *trustedSubnet != "" {
		cfg.TrustedSubnet = *trustedSubnet
	}

	overwriteFromEnv(cfg)

	if cfg.TrustedSubnet != "" {
		_, ipnet, err := net.ParseCIDR(cfg.TrustedSubnet)
		if err != nil {
			logger.Error("invalid trusted subnet",
				zap.String("subnet", cfg.TrustedSubnet),
				zap.Error(err))
			return nil, fmt.Errorf("invalid trusted subnet %q: %w", cfg.TrustedSubnet, err)
		}
		cfg.TrustedSubnetNet = ipnet
		logger.Info("trusted subnet configured",
			zap.String("subnet", cfg.TrustedSubnet))
	} else {
		logger.Info("trusted subnet is empty - stats will be forbidden for everyone")
	}

	cfg.Normalize()
	if err := cfg.Validate(logger); err != nil {
		logger.Error("configuration validation failed", zap.Error(err))
		return nil, err
	}
	logger.Info("configuration validated successfully")

	return cfg, nil
}

// overwriteFromEnv перезаписывает значения конфигурации из переменных окружения.
//
// Переменные окружения имеют высший приоритет и переопределяют значения
// установленные через флаги командной строки.
//
// Поддерживаемые переменные окружения:
//
//	SERVER_ADDRESS           : адрес сервера
//	BASE_URL                 : базовый URL
//	FILE_STORAGE_PATH        : путь к файлу хранилища
//	DATABASE_DSN             : DSN для PostgreSQL
//	DELETION_QUEUE_BUFFER    : размер буфера очереди удаления
//	DELETION_FLUSH_INTERVAL  : интервал сброса удаления (duration string)
//	DELETION_MAX_BATCH_SIZE  : максимальный размер батча удаления
//	AUDIT_FILE               : путь к файлу аудита
//	AUDIT_URL                : URL сервера аудита
//	AUDIT_BUFFER_SIZE        : размер буфера канала аудита
//	AUDIT_SHUTDOWN_TIMEOUT   : таймаут shutdown аудита
//	DB_MAX_CONNS             : максимум соединений с БД
//	DB_MIN_CONNS             : минимум соединений с БД
//	DB_MAX_LIFETIME          : максимальное время жизни соединения
//	DB_HEALTH_PERIOD         : период проверки здоровья
//	SERVER_TIMEOUT           : таймаут HTTP сервера
//	INIT_TIMEOUT             : таймаут инициализации
//	SHUTDOWN_TIMEOUT         : таймаут graceful shutdown
//	PING_TIMEOUT             : таймаут ping БД
//	HTTP_CLIENT_TIMEOUT      : таймаут HTTP клиента
//	ENABLE_HTTPS             : включение HTTPS
//	TLS_CERT                 : путь к TLS сертификату
//	TLS_KEY                  : путь к TLS ключу
//	TRUSTED_SUBNET           : доверенная подсеть в CIDR-нотации (например, "192.168.0.0/16")
//
// Примеры duration для интервалов:
//
//	"50ms", "1s", "2m30s", "1h"
func overwriteFromEnv(cfg *Config) {
	// Srv
	if val, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		cfg.ServerAddr = val
	}
	if val, ok := os.LookupEnv("BASE_URL"); ok {
		cfg.BaseURL = val
	}
	if val, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.FileStoragePath = val
	}
	if val, ok := os.LookupEnv("DATABASE_DSN"); ok {
		cfg.DatabaseDSN = val
	}

	// Deleter
	if val, ok := os.LookupEnv("DELETION_QUEUE_BUFFER"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.DeletionQueueBuffer = i
		}
	}
	if val, ok := os.LookupEnv("DELETION_FLUSH_INTERVAL"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.DeletionFlushInterval = dur
		}
	}
	if val, ok := os.LookupEnv("DELETION_MAX_BATCH_SIZE"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.DeletionMaxBatchSize = i
		}
	}

	// Audit
	if val, ok := os.LookupEnv("AUDIT_FILE"); ok {
		cfg.AuditFilePath = val
	}

	if val, ok := os.LookupEnv("AUDIT_URL"); ok {
		cfg.AuditRemoteURL = val
	}
	if val, ok := os.LookupEnv("AUDIT_BUFFER_SIZE"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.AuditBufferSize = i
		}
	}
	if val, ok := os.LookupEnv("AUDIT_SHUTDOWN_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.AuditShutdownTimeout = dur
		}
	}
	//DB
	if val, ok := os.LookupEnv("DB_MAX_CONNS"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.DB.DBMaxConns = i
		}
	}
	if val, ok := os.LookupEnv("DB_MIN_CONNS"); ok {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.DB.DBMinConns = i
		}
	}
	if val, ok := os.LookupEnv("DB_MAX_LIFETIME"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.DB.DBMaxConnLifetime = dur
		}
	}
	if val, ok := os.LookupEnv("DB_HEALTH_PERIOD"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.DB.DBHealthCheckPeriod = dur
		}
	}
	// Timeout
	if val, ok := os.LookupEnv("SERVER_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.ServerTimeout = dur
		}
	}
	if val, ok := os.LookupEnv("INIT_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.InitTimeout = dur
		}
	}
	if val, ok := os.LookupEnv("SHUTDOWN_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.ShutdownTimeout = dur
		}
	}
	if val, ok := os.LookupEnv("PING_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.PingTimeout = dur
		}
	}
	if val, ok := os.LookupEnv("HTTP_CLIENT_TIMEOUT"); ok {
		if dur, err := time.ParseDuration(val); err == nil && dur > 0 {
			cfg.HTTPClientTimeout = dur
		}
	}
	// HTTPS
	if val, ok := os.LookupEnv("ENABLE_HTTPS"); ok {
		cfg.EnableHTTPS = val == "true" || val == "1" || val == "yes"
	}

	if val, ok := os.LookupEnv("TLS_CERT"); ok {
		cfg.TLSCert = val
	}

	if val, ok := os.LookupEnv("TLS_KEY"); ok {
		cfg.TLSKey = val
	}

	// Subnet
	if val, ok := os.LookupEnv("TRUSTED_SUBNET"); ok {
		cfg.TrustedSubnet = val
	}
}

// parseDuration парсит строку duration с fallback значением.
//
// Используется для парсинга длительностей из флагов командной строки.
// Если парсинг неудачен или значение некорректно, возвращается fallback.
//
// Параметры:
//   - s: строка для парсинга (например, "50ms", "1s", "2m30s")
//   - fallback: значение по умолчанию если парсинг неудачен
//
// Возвращает:
//   - time.Duration: распарсенное значение или fallback
func parseDuration(s string, fallback time.Duration) time.Duration {
	if dur, err := time.ParseDuration(s); err == nil && dur > 0 {
		return dur
	}
	return fallback
}

// Validate проверяет корректность конфигурации.
// Возвращает ошибку если конфигурация невалидна.
func (c *Config) Validate(logger *zap.Logger) error {
	if c.EnableHTTPS {
		if c.TLSCert == "" {
			logger.Error("validation failed: TLS cert missing when HTTPS enabled")
			return fmt.Errorf("TLS enabled but TLS cert is missing")
		}
		if c.TLSKey == "" {
			logger.Error("validation failed: TLS key missing when HTTPS enabled")
			return fmt.Errorf("TLS enabled but TLS key is missing")
		}
	}
	if c.ServerAddr == "" {
		logger.Error("validation failed: server address missing")
		return fmt.Errorf("server address is missing")
	}

	if c.TrustedSubnet != "" && c.TrustedSubnetNet == nil {
		return fmt.Errorf("trusted_subnet %q is set but could not be parsed as valid CIDR", c.TrustedSubnet)
	}

	logger.Debug("configuration validated successfully")
	return nil
}

// Normalize приводит конфигурацию к каноническому виду.
// Например, заменяет http:// на https:// в BaseURL если включен HTTPS.
func (c *Config) Normalize() {
	if !c.EnableHTTPS {
		return
	}

	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		return
	}

	lower := strings.ToLower(base)

	if strings.HasPrefix(lower, "https://") {
		return
	}

	if strings.HasPrefix(lower, "http://") {
		c.BaseURL = "https://" + base[len("http://"):]
		return
	}

	c.BaseURL = "https://" + base
}

// loadFromJSON загружает конфигурацию из JSON файла.
// Параметры из JSON имеют приоритет над дефолтами, но уступают флагам и env.
func loadFromJSON(path string, cfg *Config, logger *zap.Logger) error {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("cannot read config file", zap.String("path", path), zap.Error(err))
		return nil
	}

	var fileCfg struct {
		// Srv
		ServerAddr      string `json:"server_addr"`
		BaseURL         string `json:"base_url"`
		FileStoragePath string `json:"file_storage_path"`
		DatabaseDSN     string `json:"database_dsn"`
		// Deleter
		DeletionQueueBuffer   *int   `json:"deletion_queue_buffer"`
		DeletionFlushInterval string `json:"deletion_flush_interval"`
		DeletionMaxBatchSize  *int   `json:"deletion_max_batch_size"`
		// Audit
		AuditFilePath        string `json:"audit_file_path"`
		AuditRemoteURL       string `json:"audit_remote_url"`
		AuditBufferSize      *int   `json:"audit_buffer_size"`
		AuditShutdownTimeout string `json:"audit_shutdown_timeout"`
		// db
		DBMaxConns            *int   `json:"db_max_conns"`
		DBMinConns            *int   `json:"db_min_conns"`
		DBMaxConnLifetime     string `json:"db_max_conn_lifetime"`
		DBHealthCheckInterval string `json:"db_health_check_interval"`
		// Timeout
		ServerTimeout     string `json:"server_timeout"`
		InitTimeout       string `json:"init_timeout"`
		ShutdownTimeout   string `json:"shutdown_timeout"`
		PingTimeout       string `json:"ping_timeout"`
		HTTPClientTimeout string `json:"http_client_timeout"`
		// HTTPS
		EnableHTTPS *bool  `json:"enable_https"`
		TLSCert     string `json:"tls_cert"`
		TLSKey      string `json:"tls_key"`
		// Subnet
		TrustedSubnet string `json:"trusted_subnet"`
	}

	if err := json.Unmarshal(data, &fileCfg); err != nil {
		logger.Error("invalid JSON format", zap.String("path", path), zap.Error(err))
		return err
	}

	logger.Debug("JSON config parsed", zap.String("path", path))

	// Srv
	if fileCfg.ServerAddr != "" {
		cfg.ServerAddr = fileCfg.ServerAddr
	}
	if fileCfg.BaseURL != "" {
		cfg.BaseURL = fileCfg.BaseURL
	}
	if fileCfg.FileStoragePath != "" {
		cfg.FileStoragePath = fileCfg.FileStoragePath
	}
	if fileCfg.DatabaseDSN != "" {
		cfg.DatabaseDSN = fileCfg.DatabaseDSN
	}
	// Deleter
	if fileCfg.DeletionQueueBuffer != nil {
		cfg.DeletionQueueBuffer = *fileCfg.DeletionQueueBuffer
	}
	if fileCfg.DeletionFlushInterval != "" {
		dur, err := time.ParseDuration(fileCfg.DeletionFlushInterval)
		if err != nil {
			logger.Error("deletion_flush_interval invalid", zap.Error(err))
			return err
		}
		cfg.DeletionFlushInterval = dur
	}
	if fileCfg.DeletionMaxBatchSize != nil {
		cfg.DeletionMaxBatchSize = *fileCfg.DeletionMaxBatchSize
	}
	// Audit
	if fileCfg.AuditFilePath != "" {
		cfg.AuditFilePath = fileCfg.AuditFilePath
	}
	if fileCfg.AuditRemoteURL != "" {
		cfg.AuditRemoteURL = fileCfg.AuditRemoteURL
	}
	if fileCfg.AuditBufferSize != nil {
		cfg.AuditBufferSize = *fileCfg.AuditBufferSize
	}
	if fileCfg.AuditShutdownTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.AuditShutdownTimeout)
		if err != nil {
			logger.Error("audit_shutdown_timeout invalid", zap.Error(err))
			return err
		}
		cfg.AuditShutdownTimeout = dur
	}
	// DB
	if fileCfg.DBMaxConns != nil {
		cfg.DB.DBMaxConns = *fileCfg.DBMaxConns
	}
	if fileCfg.DBMinConns != nil {
		cfg.DB.DBMinConns = *fileCfg.DBMinConns
	}
	if fileCfg.DBMaxConnLifetime != "" {
		dur, err := time.ParseDuration(fileCfg.DBMaxConnLifetime)
		if err != nil {
			logger.Error("db_max_conn_lifetime invalid", zap.Error(err))
			return err
		}
		cfg.DB.DBMaxConnLifetime = dur
	}
	if fileCfg.DBHealthCheckInterval != "" {
		dur, err := time.ParseDuration(fileCfg.DBHealthCheckInterval)
		if err != nil {
			logger.Error("db_health_check_interval invalid", zap.Error(err))
			return err
		}
		cfg.DB.DBHealthCheckPeriod = dur
	}
	// Timeout
	if fileCfg.ServerTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.ServerTimeout)
		if err != nil {
			logger.Error("server_timeout invalid", zap.Error(err))
			return err
		}
		cfg.ServerTimeout = dur
	}
	if fileCfg.InitTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.InitTimeout)
		if err != nil {
			logger.Error("init_timeout invalid", zap.Error(err))
			return err
		}
		cfg.InitTimeout = dur
	}
	if fileCfg.ShutdownTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.ShutdownTimeout)
		if err != nil {
			logger.Error("shutdown_timeout invalid", zap.Error(err))
			return err
		}
		cfg.ShutdownTimeout = dur
	}
	if fileCfg.PingTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.PingTimeout)
		if err != nil {
			logger.Error("ping_timeout invalid", zap.Error(err))
			return err
		}
		cfg.PingTimeout = dur
	}
	// HTTPS
	if fileCfg.HTTPClientTimeout != "" {
		dur, err := time.ParseDuration(fileCfg.HTTPClientTimeout)
		if err != nil {
			logger.Error("client_timeout invalid", zap.Error(err))
			return err
		}
		cfg.HTTPClientTimeout = dur
	}

	if fileCfg.EnableHTTPS != nil {
		cfg.EnableHTTPS = *fileCfg.EnableHTTPS
	}
	if fileCfg.TLSCert != "" {
		cfg.TLSCert = fileCfg.TLSCert
	}
	if fileCfg.TLSKey != "" {
		cfg.TLSKey = fileCfg.TLSKey
	}

	if fileCfg.TrustedSubnet != "" {
		cfg.TrustedSubnet = fileCfg.TrustedSubnet
	}

	return nil

}
