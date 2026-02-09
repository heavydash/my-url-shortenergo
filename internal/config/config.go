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
		AuditFilePath:  *auditFileFlag,
		AuditRemoteURL: *auditURLFlag,
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
//	SERVER_ADDRESS          : адрес сервера
//	BASE_URL               : базовый URL
//	FILE_STORAGE_PATH      : путь к файлу хранилища
//	DATABASE_DSN           : DSN для PostgreSQL
//	DELETION_QUEUE_BUFFER  : размер буфера очереди удаления
//	DELETION_FLUSH_INTERVAL: интервал сброса удаления (duration string)
//	DELETION_MAX_BATCH_SIZE: максимальный размер батча удаления
//	AUDIT_FILE             : путь к файлу аудита
//	AUDIT_URL              : URL сервера аудита
//
// Примеры duration для DELETION_FLUSH_INTERVAL:
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
