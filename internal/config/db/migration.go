package db

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
    CREATE TABLE IF NOT EXISTS urls (
      uuid         TEXT PRIMARY KEY,
      short_url       TEXT UNIQUE NOT NULL,
      original_url     TEXT NOT NULL,
      created_at       TIMESTAMPTZ DEFAULT NOW()
        );
    CREATE INDEX IF NOT EXISTS idx_short_url ON urls(short_url);
`
	_, err := pool.Exec(ctx, query)
	return err
}
