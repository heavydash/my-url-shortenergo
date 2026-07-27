// Package repository предоставляет реализации хранилищ для сервиса сокращения URL.
//
// PostgresRepository — реализация, использующая PostgreSQL с pgxpool.
// Поддерживает транзакции, batch-операции, soft delete и оптимизированные запросы.
package repository

import (
	"context"
	"errors"
	"fmt"
	idGen "github.com/heavydash/my-url-shortenergo/internal/generator"
	urlGen "github.com/heavydash/my-url-shortenergo/internal/generator"
	"time"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

// DeleteTask представляет задачу на удаление URL (используется в worker'ах).
// Сохраняется для обратной совместимости и возможного расширения
type DeleteTask struct {
	UserID    uuid.UUID
	ShortURLs []string
}

// PostgresRepository реализует хранилище URL в PostgreSQL базе данных.
//
// Особенности:
//   - Использует pgx/pgxpool для эффективного подключения
//   - Поддерживает транзакции и batch операции
//   - Реализует optimistic concurrency control
//   - Поддерживает soft delete с временными метками
//   - Использует connection pooling для производительности
//
// Архитектура:
//   - Подключение: pgxpool.Pool для управления соединениями
//   - Транзакции: поддержка ACID операций
//   - Batch операции: эффективная вставка множества записей
//   - Конфликт: ON CONFLICT DO NOTHING с последующим поиском
//
// Используется для:
//   - High availability систем
//   - Систем с большим объемом данных
//   - Распределенных приложений
type PostgresRepository struct {
	pool    *pgxpool.Pool
	logger  *zap.Logger
	baseURL string
}

// NewPostgresRepository создает новый PostgreSQL репозиторий.
//
// Инициализирует репозиторий с готовым пулом соединений.
// Предполагается что пул уже настроен (максимальные соединения, таймауты).
//
// Параметры:
//   - pool: настроенный пул соединений pgxpool.Pool
//   - logger: логгер для записи операционных событий
//   - baseURL: базовый URL для формирования полных коротких ссылок
//
// Возвращает:
//   - *PostgresRepository: готовый к использованию репозиторий
//
// Пример использования:
//
//	pool, err := pgxpool.New(context.Background(), connString)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	repo := repository.NewPostgresRepository(pool, logger)
//	defer repo.Close()
//
// Примечания:
//   - Рекомендуется использовать connection pooling в production
//   - max_connections в соответствии с нагрузкой
//   - Close() при завершении работы
func NewPostgresRepository(pool *pgxpool.Pool, logger *zap.Logger, baseURL string) *PostgresRepository {
	return &PostgresRepository{
		pool:    pool,
		logger:  logger,
		baseURL: baseURL,
	}
}

// SaveURL сохраняет URL в PostgreSQL базу данных.
//
// Выполняет:
//  1. Попытку вставки с генерацией нового UUID
//  2. При конфликте по original_url (ON CONFLICT DO NOTHING) - поиск существующей записи
//  3. Возврат существующей записи с ошибкой ErrConflict
//
// Параметры:
//   - ctx: контекст для отмены операции и таймаутов
//   - m: модель URL для сохранения (поле ID игнорируется, генерируется заново)
//
// Возвращает:
//   - *model.URLModel: указатель на сохранённую модель
//   - error: ErrConflict если URL уже существует, иначе ошибка базы данных
//
// Пример использования:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	url := model.URLModel{
//	    OriginalURL: "https://example.com",
//	    UserID:      userID,
//	}
//	savedURL, err := repo.SaveURL(ctx, url)
//	if errors.Is(err, repository.ErrConflict) {
//	    // URL уже существует, используем savedURL (существующую запись)
//	}
//
// SQL логика:
//
//	INSERT ... ON CONFLICT (original_url) DO NOTHING
//	- При успехе: возвращает сгенерированный ID
//	- При конфликте: возвращает ErrNoRows, выполняется SELECT для поиска существующей записи

func (p *PostgresRepository) SaveURL(ctx context.Context, m model.URLModel) (*model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Если ShortURL не передан, генерируем его
	if m.ShortURL == "" {
		shortURL, err := idGen.IDGen()
		if err != nil {
			return nil, fmt.Errorf("failed to generate short URL: %w", err)
		}
		m.ShortURL = shortURL
	}

	// Если ID не передан, генерируем новый UUID
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}

	query := `
		INSERT INTO urls (id, short_url, original_url, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (original_url) DO NOTHING
		RETURNING id, short_url
	`

	var returnedID uuid.UUID
	var returnedShortURL string

	err := p.pool.QueryRow(ctx, query,
		m.ID,       // используем сгенерированный или переданный ID
		m.ShortURL, // используем сгенерированный или переданный ShortURL
		m.OriginalURL,
		m.UserID,
	).Scan(&returnedID, &returnedShortURL)

	if err == nil {
		// Успешно вставили
		m.ID = returnedID
		m.ShortURL = returnedShortURL

		p.logger.Info("URL successfully saved to PostgreSQL",
			zap.String("short_url", m.ShortURL),
			zap.String("original_url", m.OriginalURL))

		saved := m
		return &saved, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// Конфликт ищем существующую запись
		var existing model.URLModel
		err = p.pool.QueryRow(ctx,
			`SELECT id, short_url, original_url, user_id, is_deleted FROM urls WHERE original_url = $1`,
			m.OriginalURL,
		).Scan(&existing.ID, &existing.ShortURL, &existing.OriginalURL, &existing.UserID,
			&existing.IsDeleted)

		if err != nil {
			p.logger.Error("Failed to lookup existing URL after conflict",
				zap.String("original_url", m.OriginalURL),
				zap.Error(err))
			return nil, err
		}

		p.logger.Warn("URL already exists in PostgreSQL",
			zap.String("short_url", existing.ShortURL),
			zap.String("original_url", existing.OriginalURL))

		return &existing, ErrConflict
	}

	p.logger.Error("Failed to save URL to PostgreSQL",
		zap.String("original_url", m.OriginalURL),
		zap.Error(err))

	return nil, err
}

// GetURL возвращает URL по его short идентификатору.
//
// Выполняет SELECT запрос по полю short_url.
// Возвращает полную модель URL включая флаг is_deleted.
//
// Параметры:
//   - ctx: контекст для отмены операции и таймаутов
//   - id: short идентификатор URL
//
// Возвращает:
//   - *model.URLModel: указатель на найденный URL (nil если не найден)
//   - error: "url not found" если URL не существует, иначе ошибка базы данных
//
// Пример использования:
//
//	url, err := repo.GetURL(ctx, "abc123")
//	if err != nil {
//	    // URL не найден или ошибка БД
//	}
//	if url.IsDeleted {
//	    // URL помечен как удаленный
//	}
//
// Примечания:
//   - Возвращает URL даже если is_deleted = true
//   - Проверка is_deleted выполняется на уровне сервиса
//   - Чувствительность к регистру зависит от collation базы данных
func (p *PostgresRepository) GetURL(ctx context.Context, id string) (*model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var m model.URLModel

	query := `SELECT id, short_url, original_url, user_id, is_deleted FROM urls WHERE short_url = $1`

	err := p.pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.ShortURL,
		&m.OriginalURL,
		&m.UserID,
		&m.IsDeleted,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		p.logger.Debug("URL not found in PostgreSQL", zap.String("short_id", id))
		return nil, fmt.Errorf("url not found")
	}
	if err != nil {

		p.logger.Error("Failed to get URL from PostgreSQL",
			zap.String("short_id", id),
			zap.Error(err))

		return nil, err
	}
	return &m, nil
}

// SaveBatch сохраняет несколько URL в рамках одной транзакции.
//
// Выполняет:
//  1. Проверку контекста на отмену
//  2. Начало транзакции
//  3. Для каждой записи в батче:
//     - Генерацию short_url через idgen.ShortURLGen(), если он не передан
//     - Генерацию UUID, если он не передан
//     - Проверку на дубликаты short_url внутри батча
//  4. Выполнение batch вставки
//  5. Коммит транзакции
//
// Параметры:
//   - ctx: контекст для cancellation/timeout транзакции
//   - batch: слайс URLModel для сохранения
//
// Возвращает:
//   - error: ошибка если транзакция не удалась или произошла ошибка генерации/вставки
//
// Пример использования:
//
//	urls := []model.URLModel{
//	    {OriginalURL: "https://example1.com", UserID: userID},
//	    {OriginalURL: "https://example2.com", UserID: userID},
//	}
//	err := repo.SaveBatch(ctx, urls)
//	if err != nil {
//	    // откат транзакции выполнен автоматически
//	}
//
// Особенности:
//   - Использует транзакцию для атомарности
//   - Генерирует уникальные short_url для каждой записи
//   - Защищает от дубликатов внутри одного батча
//   - При ошибке выполняется автоматический Rollback
func (p *PostgresRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		p.logger.Error("Failed to begin transaction for SaveBatch", zap.Error(err))
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	b := &pgx.Batch{}
	seen := make(map[string]bool) // защита от дубликатов short_url внутри батча

	for i := range batch {
		m := &batch[i]

		// Генерируем ShortURL, если не передан
		if m.ShortURL == "" {
			for attempt := 0; attempt < 5; attempt++ {
				shortURL, err := urlGen.ShortURLGen()
				if err != nil {
					p.logger.Error("Failed to generate short URL in batch", zap.Error(err))
					return fmt.Errorf("failed to generate short URL: %w", err)
				}
				if !seen[shortURL] {
					m.ShortURL = shortURL
					seen[shortURL] = true
					break
				}
			}
		} else if seen[m.ShortURL] {
			return fmt.Errorf("duplicate short_url in batch: %s", m.ShortURL)
		} else {
			seen[m.ShortURL] = true
		}

		// Генерируем UUID, если не передан
		if m.ID == uuid.Nil {
			m.ID = uuid.New()
		}

		b.Queue("INSERT INTO urls (id, short_url, original_url, user_id) VALUES ($1, $2, $3, $4)",
			m.ID, m.ShortURL, m.OriginalURL, m.UserID)
	}

	br := tx.SendBatch(ctx, b)

	// Проверяем результат каждой операции в батче
	for i := 0; i < len(batch); i++ {
		_, err := br.Exec()
		if err != nil {
			p.logger.Error("Batch insert failed for item",
				zap.Int("index", i),
				zap.Error(err))
			return fmt.Errorf("failed to insert item %d: %w", i, err)
		}
	}

	if err = br.Close(); err != nil {
		p.logger.Error("Failed to close batch", zap.Error(err))
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		p.logger.Error("Failed to commit batch insert", zap.Error(err))
		return err
	}

	committed = true
	p.logger.Info("Batch URLs successfully saved to PostgreSQL", zap.Int("count", len(batch)))
	return nil
}

// Ping проверяет доступность PostgreSQL базы данных.
//
// Выполняет простой ping запрос к базе данных.
// Используется для health checks и мониторинга.
//
// Параметры:
//   - ctx: контекст с таймаутом для проверки
//
// Возвращает:
//   - error: ошибка если база данных недоступна
//
// Пример использования:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
//	defer cancel()
//	if err := repo.Ping(ctx); err != nil {
//	    log.Println("Database is down")
//	}
//
// Примечания:
//   - Рекомендуется использовать с таймаутом 1-5 секунд
func (p *PostgresRepository) Ping(ctx context.Context) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := p.pool.Ping(ctx)
	if err != nil {
		p.logger.Warn("PostgreSQL ping failed", zap.Error(err))
	}
	return err
}

// Clear полностью очищает таблицу urls.
//
// Выполняет TRUNCATE TABLE urls - удаляет все записи.
// Не использует CASCADE, не затрагивает другие таблицы.
//
// Возвращает:
//   - error: ошибка операции TRUNCATE
//
// Используется для:
//   - Интеграционных тестов
//   - Development окружения
//   - Аварийного сброса данных
//
// Предупреждение:
//   - Операция необратима
//   - Не использовать в production без backup
//   - TRUNCATE быстрее чем DELETE FROM urls
func (p *PostgresRepository) Clear() error {
	_, err := p.pool.Exec(context.Background(), "TRUNCATE TABLE urls")
	if err != nil {
		p.logger.Error("Failed to truncate urls table in PostgreSQL", zap.Error(err))
		return err
	}
	p.logger.Info("URLs table successfully truncated in PostgreSQL")
	return nil
}

// GetURLsByUser возвращает все неудаленные URL принадлежащие пользователю.
//
// Выполняет SELECT с фильтрацией по user_id и is_deleted.
// Преобразует short_url в полные URL путем конкатенации с baseURL.
// Сортирует по created_at DESC (последние созданные сначала).
//
// Параметры:
//   - ctx: контекст для cancellation/timeout запроса
//   - userID: UUID пользователя
//
// Возвращает:
//   - []model.URLModel: слайс URL пользователя с полными shortURL
//   - error: ошибка выполнения запроса
//
// Пример использования:
//
//	urls, err := repo.GetURLsByUser(ctx, userID)
//	for _, url := range urls {
//	    // url.ShortURL: "http://localhost:8080/abc123"
//	    // url.OriginalURL: "https://example.com"
//	}
//
// Примечания:
//   - Для uuid.Nil возвращает пустой слайс
//   - BaseURL берется из конфигурации и передается при создании репозитория
//   - Возвращает только URL с is_deleted = false
func (p *PostgresRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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

		p.logger.Error("Failed to get user URLs from PostgreSQL",
			zap.String("user_id", userID.String()),
			zap.Error(err))

		return nil, err
	}
	defer rows.Close()

	var urls []model.URLModel

	for rows.Next() {
		var u model.URLModel
		var deleted bool
		if err := rows.Scan(&u.ID, &u.ShortURL, &u.OriginalURL, &deleted); err != nil {
			p.logger.Error("Failed to scan row in GetURLsByUser", zap.Error(err))
			return nil, err
		}
		if !deleted {
			u.ShortURL = fmt.Sprintf("%s/%s", p.baseURL, u.ShortURL)
			urls = append(urls, u)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	p.logger.Debug("Retrieved user URLs from PostgreSQL",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(urls)))

	return urls, nil
}

// MarkAsDeleted помечает URL как удаленные (soft delete) для указанного пользователя.
//
// Выполняет batch update с использованием pq.Array для передачи массива shortURL.
// Разбивает большой массив на батчи по 500 элементов для предотвращения ошибок
// максимального количества параметров.
// Устанавливает is_deleted = TRUE и deleted_at = NOW().
//
// Параметры:
//   - ctx: контекст для отмены операции и таймаутов
//   - userID: UUID пользователя для проверки владения
//   - shortURLs: слайс short идентификаторов для пометки как удаленные
//
// Возвращает:
//   - error: ошибка выполнения update запроса
//
// Пример использования:
//
//	err := repo.MarkAsDeleted(ctx, userID, []string{"abc123", "def456"})
//	if err != nil {
//	    // ошибка обновления БД
//	}
//
// Особенности:
//   - Использует ANY($2) для сравнения с массивом
//   - Батчинг для больших списков (по 500 элементов)
//   - Проверяет is_deleted = FALSE чтобы избежать лишних обновлений
//   - Логирует процесс для отладки
func (p *PostgresRepository) MarkAsDeleted(ctx context.Context, userID uuid.UUID, shortURLs []string) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if len(shortURLs) == 0 {
		return nil
	}

	const batchSize = 500

	for i := 0; i < len(shortURLs); i += batchSize {
		end := i + batchSize
		if end > len(shortURLs) {
			end = len(shortURLs)
		}
		batch := shortURLs[i:end]

		query := `
		UPDATE urls 
		SET is_deleted = TRUE, deleted_at = NOW()
        WHERE user_id = $1
          AND short_url = ANY($2)
          AND is_deleted = FALSE`

		result, err := p.pool.Exec(ctx, query, userID, pq.Array(batch))
		if err != nil {
			p.logger.Error("Failed to mark URLs as deleted in PostgreSQL",
				zap.String("user_id", userID.String()),
				zap.Int("batch_size", len(batch)),
				zap.Error(err))
			return fmt.Errorf("failed to mark batch as deleted: %w", err)
		}

		affected := result.RowsAffected()

		if affected > 0 {
			p.logger.Info("URLs marked as deleted in PostgreSQL",
				zap.String("user_id", userID.String()),
				zap.Int("affected", int(affected)))
		}

	}

	p.logger.Info("Completed marking URLs as deleted",
		zap.String("user_id", userID.String()),
		zap.Int("total_requested", len(shortURLs)))

	return nil
}

// Close закрывает пул соединений с базой данных.
//
// Вызывает pool.Close() который:
//   - Завершает все активные соединения
//   - Останавливает фоновые процессы пула
//   - Освобождает ресурсы
//
// Возвращает:
//   - error: ошибка закрытия пула
//
// Пример использования:
//
//	func main() {
//	    repo := repository.NewPostgresRepository(pool, logger)
//	    defer repo.Close()
//	    // работа с репозиторием...
//	}
//
// Примечания:
//   - Всегда вызывать Close() при завершении работы приложения
//   - После Close() вызовы методов репозитория приведут к ошибкам
//   - Метод идемпотентен (безопасен для повторных вызовов)
func (p *PostgresRepository) Close() error {
	if p.pool != nil {
		p.pool.Close()
		p.logger.Info("postgres: closed pool")
	}
	return nil
}

// Stats возвращает статистику сервиса из PostgreSQL.
//
// Реализация:
//   - Выполняет один SQL-запрос:
//     SELECT COUNT(*) AS urls, COUNT(DISTINCT user_id) AS users FROM urls
//   - Использует контекст с таймаутом 5 секунд.
//   - При любой ошибке (включая отмену контекста или проблемы с соединением)
//     возвращает (0, 0) и логирует ошибку на уровне Error.
//
// Преимущества:
//   - Высокая производительность даже при миллионах записей (благодаря индексам PostgreSQL).
//   - Не блокирует приложение надолго.
//   - Масштабируется линейно с ростом базы данных.
func (p *PostgresRepository) Stats() (urls int, users int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if ctx.Err() != nil {
		p.logger.Warn("Stats canceled by context", zap.Error(ctx.Err()))
		return 0, 0
	}

	query := `SELECT COUNT(*) AS urls,
       COUNT(DISTINCT user_id) AS users FROM urls`

	err := p.pool.QueryRow(ctx, query).Scan(&urls, &users)
	if err != nil {
		p.logger.Error("Failed to get stats from PostgreSQL", zap.Error(err))
		return 0, 0
	}
	return urls, users
}
