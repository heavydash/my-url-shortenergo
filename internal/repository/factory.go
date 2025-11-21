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
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		pool, err := pgxpool.New(ctx, cfg.DatabaseDSN)
		if err != nil {
			logger.Warn("postgres unavailable, using file storage", zap.Error(err))
		} else {
			// Создаём таблицу вручную
			_, err = pool.Exec(context.Background(), `
            CREATE TABLE IF NOT EXISTS urls (
                uuid TEXT PRIMARY KEY,
                short_url TEXT UNIQUE NOT NULL,
                original_url TEXT NOT NULL,
                user_id TEXT
            )
        `)
			if err != nil {
				logger.Error("failed to create table", zap.Error(err))
			} else {
				return NewPostgres(pool)
			}
		}
	}
	if cfg.FileStoragePath != "" {
		return NewFileRepository(cfg.FileStoragePath)
	}
	return NewMemoryRepository()
}
