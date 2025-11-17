package repository

import (
	"context"
	"github.com/golang-migrate/migrate/v4"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"go.uber.org/zap"
)

func New(cfg *config.Config, logger *zap.Logger) URLRepository {
	ctx := context.Background()

	if cfg.DatabaseDSN != "" {
		pool, err := db.New(ctx, cfg.DatabaseDSN)
		if err != nil {
			logger.Fatal("failed to connect to database", zap.Error(err))
		}
		//Миграции
		m, err := migrate.New("file://migrations", cfg.DatabaseDSN)
		if err != nil {
			logger.Fatal("migrate new", zap.Error(err))
		}
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			logger.Fatal("migrate up", zap.Error(err))
		}
		return NewPostgres(pool)
	}

	if cfg.FileStoragePath != "" {
		return NewFileRepository(cfg.FileStoragePath)
	}

	return NewMemoryRepository()
}
