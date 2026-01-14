package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServerAddr      string
	BaseURL         string
	FileStoragePath string
	DatabaseDSN     string

	// Поля для Deleter'а
	DeletionQueueBuffer   int           `env:"DELETION_QUEUE_BUFFER"`
	DeletionFlushInterval time.Duration `env:"DELETION_FLUSH_INTERVAL"`
	DeletionMaxBatchSize  int           `env:"DELETION_MAX_BATCH_SIZE"`
}

func NewConfig() (*Config, error) {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)

	// Старые флаги
	a := fs.String("a", ":8080", "address to run HTTP server")
	b := fs.String("b", "http://localhost:8080", "base URL for shortened links")
	f := fs.String("f", "", "file path to store the URL")
	d := fs.String("d", "", "DSN to store the URL")

	// Флаги для Deleter'а
	dq := fs.Int("dq", 1000, "deletion queue buffer size (default 1000)")
	df := fs.String("df", "50ms", "deletion flush interval, e.g. 50ms, 1s (default 50ms)")
	dm := fs.Int("dm", 1000, "deletion max batch size per user (default 1000)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerAddr:            *a,
		BaseURL:               *b,
		FileStoragePath:       *f,
		DatabaseDSN:           *d,
		DeletionQueueBuffer:   *dq,
		DeletionFlushInterval: parseDuration(*df, 50*time.Millisecond),
		DeletionMaxBatchSize:  *dm,
	}

	// Перезапись env-переменными (приоритет env)
	if addr, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		cfg.ServerAddr = addr
	}
	if base, ok := os.LookupEnv("BASE_URL"); ok {
		cfg.BaseURL = base
	}
	if path, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.FileStoragePath = path
	}
	if dsn, ok := os.LookupEnv("DATABASE_DSN"); ok {
		cfg.DatabaseDSN = dsn
	}

	// Новые env
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

	return cfg, nil
}

// Вспомогательная функция для парсинга duration из флага (с fallback)
func parseDuration(s string, fallback time.Duration) time.Duration {
	if dur, err := time.ParseDuration(s); err == nil && dur > 0 {
		return dur
	}
	return fallback
}
