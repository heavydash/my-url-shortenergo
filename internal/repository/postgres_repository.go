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

func (r *PostgresRepository) SaveURL(m model.URLModel) (model.URLModel, error) {
	query := `
		INSERT INTO urls (uuid, short_url, original_url, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (original_url) DO NOTHING
		RETURNING uuid, short_url
	`

	var returnedUUID, returnedShortURL string
	err := r.pool.QueryRow(context.Background(), query,
		m.UUID, m.ShortURL, m.OriginalURL, m.UserID,
	).Scan(&returnedUUID, &returnedShortURL)

	if err == nil {
		// Успешно вставили
		m.UUID = returnedUUID
		m.ShortURL = returnedShortURL
		return m, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Конфликт ищем существующий
		var existing model.URLModel
		err = r.pool.QueryRow(context.Background(),
			`SELECT uuid, short_url, original_url, user_id FROM urls WHERE original_url = $1`,
			m.OriginalURL,
		).Scan(&existing.UUID, &existing.ShortURL, &existing.OriginalURL, &existing.UserID)

		if err != nil {
			return model.URLModel{}, err
		}

		return existing, ErrConflict
	}

	return model.URLModel{}, err
}
func (r *PostgresRepository) GetURL(id string) (model.URLModel, error) {
	var m model.URLModel
	query := `SELECT uuid, short_url, original_url, user_id FROM urls WHERE uuid = $1`
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
