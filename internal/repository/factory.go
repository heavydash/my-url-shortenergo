package repository

import (
	"context"
	"time"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewFactory(cfg *config.Config, logger *zap.Logger) URLRepository {
	if cfg.DatabaseDSN != "" {
		// Контекст для подключения
		ctxConnect, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pool, err := pgxpool.New(ctxConnect, cfg.DatabaseDSN)
		if err != nil {
			logger.Warn("postgres unavailable, using file storage", zap.Error(err))
		} else {
			// Контекст для создания таблицы или миграций
			ctxInit, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelInit()
			// Проверка Ты жива?
			if err := pool.Ping(ctxInit); err != nil {
				logger.Error("failed to ping db, falling back", zap.Error(err))
				pool.Close()
			} else {
				//Создаем таблицу
				_, err = pool.Exec(ctxInit, `
    CREATE TABLE IF NOT EXISTS urls (
        uuid TEXT PRIMARY KEY,
        short_url TEXT UNIQUE NOT NULL,
        original_url TEXT NOT NULL,
        user_id TEXT
    )
`)
				if err != nil {
					logger.Error("failed to create table, falling back", zap.Error(err))
					pool.Close()
				} else {
					logger.Info("using postgres storage")
					return NewPostgres(pool)
				}
			}
		}
	}
	// Fallback
	if cfg.FileStoragePath != "" {
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		return NewFileRepository(cfg.FileStoragePath)
	}
	logger.Info("using in-memory storage")
	return NewMemoryRepository()
}
