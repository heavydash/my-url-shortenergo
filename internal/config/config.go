// Package config предоставляет загрузку и управление конфигурацией приложения.
// Поддерживает multiple источников конфигурации: флаги командной строки и переменные окружения
// с приоритетом переменных окружения (env overrides flags).
package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

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

	// Поля для конфигурации пула соединений PostgreSQL
	//
	// Настройки пула влияют на производительность и потребление ресурсов:
	// - MaxConns: лимит одновременных соединений (предотвращает перегрузку БД)
	// - MinConns: поддержка минимального пула (снижает задержки при пиковых нагрузках)
	// - MaxConnLifetime: ротация соединений (предотвращает утечки памяти)
	// - HealthCheckPeriod: регулярная проверка доступности (быстрое обнаружение сбоев)

	// DBMaxConns - максимальное количество соединений в пуле PostgreSQL.
	//
	// Ограничивает максимальное число одновременных соединений с БД.
	// При превышении лимита запросы будут ждать освобождения соединения.
	//
	// Значение зависит от:
	//   - Лимита соединений в PostgreSQL (max_connections)
	//   - Доступной памяти на сервере
	//   - Ожидаемой конкурентной нагрузки
	//
	// Рекомендации:
	//   - Для небольших проектов: 10-20
	//   - Для средних нагрузок: 20-50
	//   - Для высоких нагрузок: 50-100 (с учётом лимитов БД)
	//
	// Флаг: -db-max-conns, env: DB_MAX_CONNS, default: 20
	DBMaxConns int

	// DBMinConns - минимальное количество соединений в пуле PostgreSQL.
	//
	// Поддерживает указанное число постоянных соединений, даже при отсутствии нагрузки.
	// Снижает задержки при резких всплесках трафика, т.к. соединения уже открыты.
	//
	// Влияние:
	//   - Слишком высокое значение: лишнее потребление ресурсов БД
	//   - Слишком низкое значение: задержки при создании новых соединений
	//
	// Рекомендации:
	//   - Для постоянной нагрузки: 5-10
	//   - Для переменной нагрузки: 2-5
	//   - Для тестов/разработки: 1-2
	//
	// Флаг: -db-min-conns, env: DB_MIN_CONNS, default: 5
	DBMinConns int

	// DBMaxConnLifetime - максимальное время жизни соединения с PostgreSQL.
	//
	// Определяет, как долго соединение может существовать до принудительного закрытия.
	// После закрытия создаётся новое соединение для поддержания пула.
	//
	// Зачем нужно:
	//   - Ротация соединений для балансировки нагрузки
	//   - Защита от утечек памяти на стороне БД
	//   - Обновление конфигурации сессии
	//
	// Рекомендации:
	//   - Для стабильных окружений: 30-60 минут
	//   - При частых изменениях схемы: 5-15 минут
	//   - По умолчанию: 5 минут (хороший баланс)
	//
	// Формат: число с единицей измерения (ms, s, m, h)
	// Примеры: "5m", "30m", "1h"
	// Флаг: -db-max-lifetime, env: DB_MAX_LIFETIME, default: "5m"
	DBMaxConnLifetime time.Duration

	// DBHealthCheckPeriod - периодичность проверки здоровья соединений в пуле.
	//
	// Фоновый процесс регулярно проверяет доступность соединений и
	// закрывает проблемные. Новые запросы будут использовать здоровые соединения.
	//
	// Влияние на производительность:
	//   - Частые проверки: лишняя нагрузка на БД
	//   - Редкие проверки: долгое обнаружение сбоев
	//
	// Рекомендации:
	//   - Для production: 1-5 минут
	//   - Для критичных систем: 30-60 секунд
	//   - Для разработки: можно увеличить до 10-15 минут
	//
	// Формат: число с единицей измерения (ms, s, m, h)
	// Примеры: "1m", "30s", "5m"
	// Флаг: -db-health-period, env: DB_HEALTH_PERIOD, default: "1m"
	DBHealthCheckPeriod time.Duration
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
func NewConfig() (*Config, error) {
	fs := flag.NewFlagSet("url-shortener", flag.ContinueOnError)

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
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerAddr:      *a,
		BaseURL:         *b,
		FileStoragePath: *f,
		DatabaseDSN:     *d,
		// Deleter
		DeletionQueueBuffer:   *dq,
		DeletionFlushInterval: parseDuration(*df, 50*time.Millisecond),
		DeletionMaxBatchSize:  *dm,
		// Audit
		AuditFilePath:        *auditFileFlag,
		AuditRemoteURL:       *auditURLFlag,
		AuditBufferSize:      *auditBufferSize,
		AuditShutdownTimeout: *auditShutdownTimeout,
		//db
		DBMaxConns:          *dbMaxConns,
		DBMinConns:          *dbMinConns,
		DBMaxConnLifetime:   *dbMaxLifetime,
		DBHealthCheckPeriod: *dbHealthPeriod,
		// Timeout
		ServerTimeout:     *serverTimeoutFlag,
		InitTimeout:       *initTimeoutFlag,
		ShutdownTimeout:   *shutdownTimeoutFlag,
		PingTimeout:       *pingTimeoutFlag,
		HTTPClientTimeout: *httpClientTimeout,
	}

	overwriteFromEnv(cfg)
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
//
// Примеры duration для интервалов:
//
//	"50ms", "1s", "2m30s", "1h"
func overwriteFromEnv(cfg *Config) {

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

	// Аудит
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

	// Таймауты
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
