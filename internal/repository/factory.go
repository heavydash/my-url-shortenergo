package repository

import (
	"context"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewFactory(cfg *config.Config, logger *zap.Logger) URLRepository {
	ctx := context.Background()

	if cfg.DatabaseDSN != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseDSN)
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
