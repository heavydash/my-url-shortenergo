package repository

import (
	"context"
	"time"

	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"go.uber.org/zap"
)

func NewFactory(cfg *config.Config, logger *zap.Logger) URLRepository {
	if cfg.DatabaseDSN != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pool, err := db.New(ctx, cfg.DatabaseDSN)
		if err != nil {
			logger.Warn("postgres unavailable, falling back to file/memory", zap.Error(err))
		} else {
			logger.Info("using postgres storage")
			return NewPostgres(pool.Pool, logger)
		}
	}

	if cfg.FileStoragePath != "" {
		logger.Info("using file storage", zap.String("path", cfg.FileStoragePath))
		return NewFileRepository(cfg.FileStoragePath)
	}

	logger.Info("using in-memory storage")
	return NewMemoryRepository()
}
