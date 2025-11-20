package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/config/db"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/pgxpool"
	"sync"
)

type PostgresRepository struct {
	mu   sync.Mutex
	Pool *db.Pool
}

func NewPostgres(pool *db.Pool) *PostgresRepository {
	return &PostgresRepository{Pool: pool}
}

func (r *PostgresRepository) SaveURL(ctx context.Context, m model.URLModel) (model.URLModel, error) {
	r.mu.Lock()
	defer r.mu.Lock()
	query := `
    INSERT INTO urls (uuid, short_url, original_url)
    VALUES ($1, $2, $3)
    ON CONFLICT (uuid) DO UPDATE SET
      short_url = EXCLUDED.short_url,
      original_url = EXCLUDED.original_url
  `
	_, err := r.Pool.Exec(context.Background(), query, m.UUID, m.ShortURL, m.OriginalURL)
	return m, err
}

func (r *PostgresRepository) GetURL(ctx context.Context, id string) (model.URLModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var m model.URLModel
	query := `SELECT uuid, short_url, original_url FROM urls WHERE uuid = $1`
	err := r.Pool.QueryRow(context.Background(), query, id).Scan(&m.UUID, &m.ShortURL, &m.OriginalURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.URLModel{}, fmt.Errorf("url not found")
	}
	if err != nil {
		return model.URLModel{}, err
	}
	return m, nil
}

func (r *PostgresRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, m := range batch {
		_, err := tx.Exec(ctx, "INSERT INTO urls (uuid, short_url, original_url)"+
			" VALUES ($1, $2, $3)", m.UUID, m.ShortURL, m.OriginalURL)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Clear() error {
	if _, err := r.Pool.Exec(context.Background(), "TRUNCATE TABLE urls"); err != nil {
		return err
	}
	return nil
}
func (r *PostgresRepository) Ping(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.Pool.Ping(ctx)
}
