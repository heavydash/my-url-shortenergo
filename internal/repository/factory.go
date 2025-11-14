package repository

import (
	"context"
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
		return NewPostgres(pool)
	}

	if cfg.FileStoragePath != "" {
		return NewFileRepository(cfg.FileStoragePath)
	}

	return NewMemoryRepository()
}
