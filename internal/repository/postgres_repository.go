package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool     *pgxpool.Pool
	initOnce sync.Once
	initErr  error
}

func NewPostgres(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) initTable(ctx context.Context) error {
	r.initOnce.Do(func() {
		_, r.initErr = r.pool.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS urls (
				uuid TEXT PRIMARY KEY,
				short_url TEXT UNIQUE NOT NULL,
				original_url TEXT NOT NULL,
				user_id TEXT,
				created_at TIMESTAMPTZ DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_short_url ON urls(short_url);
		`)
	})
	return r.initErr
}

func (r *PostgresRepository) SaveURL(m model.URLModel) (model.URLModel, error) {
	if err := r.initTable(context.Background()); err != nil {
		return model.URLModel{}, err
	}

	query := `
		INSERT INTO urls (uuid, short_url, original_url, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (uuid) DO UPDATE SET
			short_url = EXCLUDED.short_url,
			original_url = EXCLUDED.original_url,
			user_id = EXCLUDED.user_id
	`
	_, err := r.pool.Exec(context.Background(), query, m.UUID, m.ShortURL, m.OriginalURL, m.UUID)
	return m, err
}

func (r *PostgresRepository) GetURL(id string) (model.URLModel, error) {
	if err := r.initTable(context.Background()); err != nil {
		return model.URLModel{}, err
	}

	var m model.URLModel
	query := `SELECT uuid, short_url, original_url, user_id FROM urls WHERE short_url = $1`
	err := r.pool.QueryRow(context.Background(), query, id).Scan(&m.UUID, &m.ShortURL, &m.OriginalURL, &m.UUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.URLModel{}, fmt.Errorf("url not found")
	}
	if err != nil {
		return model.URLModel{}, err
	}
	return m, nil
}

func (r *PostgresRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {
	if err := r.initTable(ctx); err != nil {
		return err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	b := &pgx.Batch{}
	for _, m := range batch {
		b.Queue("INSERT INTO urls (uuid, short_url, original_url, user_id) VALUES ($1, $2, $3, $4)",
			m.UUID, m.ShortURL, m.OriginalURL, m.UUID)
	}
	br := tx.SendBatch(ctx, b)
	if err = br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *PostgresRepository) Clear() error {
	_, err := r.pool.Exec(context.Background(), "TRUNCATE TABLE urls")
	return err
}
