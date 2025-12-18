package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
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

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (p *PostgresRepository) SaveURL(m model.URLModel) (model.URLModel, error) {
	query := `
		INSERT INTO urls (id, short_url, original_url, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (original_url) DO NOTHING
		RETURNING id, short_url
	`

	var returnedID uuid.UUID
	var returnedShortURL string
	err := p.pool.QueryRow(context.Background(), query,
		uuid.New(), m.ShortURL, m.OriginalURL, m.UserID,
	).Scan(&returnedID, &returnedShortURL)

	if err == nil {
		// Успешно вставили
		m.ID = returnedID
		m.ShortURL = returnedShortURL
		return m, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Конфликт ищем существующий
		var existing model.URLModel
		err = p.pool.QueryRow(context.Background(),
			`SELECT id, short_url, original_url, user_id, is_deleted FROM urls WHERE original_url = $1`,
			m.OriginalURL,
		).Scan(&existing.ID, &existing.ShortURL, &existing.OriginalURL, &existing.UserID,
			&existing.IsDeleted)

		if err != nil {
			return model.URLModel{}, err
		}

		return existing, ErrConflict
	}

	return model.URLModel{}, err
}
func (p *PostgresRepository) GetURL(id string) (model.URLModel, error) {
	var m model.URLModel
	query := `SELECT id, short_url, original_url, user_id, is_deleted FROM urls WHERE short_url = $1`
	err := p.pool.QueryRow(context.Background(), query, id).Scan(
		&m.ID,
		&m.ShortURL,
		&m.OriginalURL,
		&m.UserID,
		&m.IsDeleted,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.URLModel{}, fmt.Errorf("url not found")
	}
	if err != nil {
		return model.URLModel{}, err
	}
	return m, nil
}

func (p *PostgresRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	b := &pgx.Batch{}
	for _, m := range batch {
		b.Queue("INSERT INTO urls (id, short_url, original_url, user_id) VALUES ($1, $2, $3, $4)",
			uuid.New(), m.ShortURL, m.OriginalURL, m.UserID)
	}
	br := tx.SendBatch(ctx, b)
	if err = br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *PostgresRepository) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PostgresRepository) Clear() error {
	_, err := p.pool.Exec(context.Background(), "TRUNCATE TABLE urls")
	return err
}

func (p *PostgresRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {
	if userID == uuid.Nil {
		return []model.URLModel{}, nil
	}

	query := `
	SELECT id, short_url, original_url, is_deleted
	FROM urls
	WHERE user_id = $1 AND is_deleted = false
	ORDER BY created_at DESC
`
	rows, err := p.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get urls by user: query failed: %w", err)
	}
	defer rows.Close()

	var urls []model.URLModel
	baseURL := "http://localhost:8080"

	for rows.Next() {
		var u model.URLModel
		var deleted bool
		if err := rows.Scan(&u.ID, &u.ShortURL, &u.OriginalURL, &deleted); err != nil {
			return nil, fmt.Errorf("get urls by user: scan failed: %w", err)
		}
		if !deleted {
			u.ShortURL = fmt.Sprintf("%s/%s", baseURL, u.ShortURL)
			urls = append(urls, u)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func (p *PostgresRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	if len(shortURLs) == 0 {
		return nil
	}

	query := `UPDATE urls SET is_deleted = true
	WHERE short_url = ANY($1) AND user_id = $2

`
	_, err := p.pool.Exec(context.Background(), query, shortURLs, userID)
	return err
}
