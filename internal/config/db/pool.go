package db

import (
	"context"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool — обёртка над pgxpool.Pool для удобства использования в проекте
type Pool struct {
	*pgxpool.Pool
}

// New создаёт пул соединений с PostgreSQL с настройками из конфигурации
//
// Параметры:
//   - ctx: контекст для создания пула
//   - dsn: строка подключения (postgres://...)
//   - cfg: конфигурация приложения (таймауты, размеры пула и т.д.)
//
// Возвращает:
//   - *Pool: готовый к использованию пул соединений
//   - error: ошибка подключения или конфигурации
//
// Пример использования:
//
//	pool, err := db.New(ctx, cfg.DatabaseDSN, cfg)
//	if err != nil {
//	    logger.Fatal("failed to connect to db", zap.Error(err))
//	}
//	defer pool.Close()
func New(ctx context.Context, dsn string, cfg *config.Config) (*Pool, error) {
	pgxCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	//Pool params
	pgxCfg.MaxConns = int32(cfg.DBMaxConns)
	pgxCfg.MinConns = int32(cfg.DBMinConns)
	pgxCfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	pgxCfg.HealthCheckPeriod = cfg.DBHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, err
	}

	// Проверка доступности базы при старте
	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Pool{Pool: pool}, nil
}

// Ping выполняет проверку соединения с использованием таймаута из конфига
//
// Параметры:
//   - ctx: контекст для отмены
//   - cfg: конфигурация (используется PingTimeout)
//
// Возвращает:
//   - error: ошибка если БД недоступна или превышен таймаут
func (p *Pool) Ping(ctx context.Context, cfg *config.Config) error {
	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	return p.Pool.Ping(pingCtx)
}

// Close закрывает пул соединений
func (p *Pool) Close() {
	p.Pool.Close()
}
