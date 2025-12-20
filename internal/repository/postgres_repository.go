package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
)

type DeleteTask struct {
	UserID    uuid.UUID
	ShortURLs []string
}

type PostgresRepository struct {
	pool     *pgxpool.Pool
	initOnce sync.Once
	initErr  error
	deleteCh chan DeleteTask
	logger   *zap.Logger
	wg       *sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewPostgresRepository(pool *pgxpool.Pool, logger *zap.Logger) *PostgresRepository {
	p := &PostgresRepository{
		pool:   pool,
		logger: logger,
		wg:     &sync.WaitGroup{},
	}
	p.deleteCh = make(chan DeleteTask, 1000)
	p.ctx, p.cancel = context.WithCancel(context.Background())

	logger.Info("STARTING SINGLE delete worker with fan-in")
	p.wg.Add(1)
	go p.deleteWorker(p.ctx)
	return p
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

func (p *PostgresRepository) deleteWorker(ctx context.Context) {
	defer p.wg.Done()

	type accumulatedTask struct {
		userID uuid.UUID
		urls   []string
	}

	var batch []accumulatedTask
	timer := time.NewTimer(100 * time.Millisecond) // таймер на сброс батча

	flush := func() {
		if len(batch) == 0 {
			return
		}
		p.logger.Info("Flushing batch", zap.Int("tasks_count", len(batch)))

		// Группируем по userID и делаем один UPDATE на юзера с большим массивом
		byUser := make(map[uuid.UUID][]string)
		for _, t := range batch {
			byUser[t.userID] = append(byUser[t.userID], t.urls...)
		}

		for uid, urls := range byUser {
			if err := p.markAsDeletedBatch(uid, urls); err != nil {
				p.logger.Error("batch delete failed", zap.Error(err))
			} else {
				p.logger.Info("Deleted for user", zap.String("user_id", uid.String()),
					zap.Int("urls_count", len(urls)))
			}
		}

		batch = batch[:0]
	}

	for {
		select {
		case task, ok := <-p.deleteCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, accumulatedTask{userID: task.UserID, urls: task.ShortURLs})

			if len(batch) >= 20 { // порог для накопления
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(100 * time.Millisecond)
			}
		case <-timer.C:
			flush()
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(100 * time.Millisecond)
		case <-p.ctx.Done(): // сброс при shut down
			flush()
			return
		}
	}
}

func (p *PostgresRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	if len(shortURLs) == 0 {
		return nil
	}
	p.logger.Info("MarkAsDeleted: sending to channel", zap.String("user_id",
		userID.String()), zap.Strings("short_urls", shortURLs))
	select {
	case p.deleteCh <- DeleteTask{UserID: userID, ShortURLs: shortURLs}:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *PostgresRepository) markAsDeletedBatch(userID uuid.UUID, shortURLs []string) error {
	p.logger.Info("markAsDeletedBatch: START",
		zap.String("user_id", userID.String()),
		zap.Int("urls_count", len(shortURLs)))

	if len(shortURLs) == 0 {
		return nil
	}

	query := `UPDATE urls 
		SET is_deleted = true
		WHERE short_url = ANY($1)
		AND is_deleted = false
    RETURNING short_url 
  		RETURNING short_url`

	tag, err := p.pool.Exec(context.Background(), query, pq.Array(shortURLs), userID)

	if err != nil {
		p.logger.Error("markAsDeletedBatch: EXEC FAILED", zap.Error(err))
		return err
	}

	rowsAffected := tag.RowsAffected()
	p.logger.Info("markAsDeletedBatch: SUCCESS",
		zap.Int64("rows_affected", rowsAffected),
		zap.Strings("short_urls", shortURLs))
	return nil
}

func (p *PostgresRepository) Close() error {
	p.cancel() // отменяем ctx

	close(p.deleteCh) // закрываем канал

	// Ждём воркер с таймаутом, чтоб не висеть бесконечно
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("repo closed gracefully")
		return nil
	case <-time.After(3 * time.Second):
		p.logger.Warn("repo close timeout, forcing close")
		return errors.New("repo close timeout")
	}
}
