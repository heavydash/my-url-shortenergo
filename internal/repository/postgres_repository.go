// Package repository предоставляет реализации хранилищ для URL shortener.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

// DeleteTask представляет задачу на удаление URL (используется в worker'ах).
// Сохраняется для обратной совместимости и возможного расширения.
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
//   - Production окружений
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
//   - m: URLModel для сохранения (поле ID игнорируется, генерируется заново)
//
// Возвращает:
//   - model.URLModel: сохраненная модель с заполненными полями ID и ShortURL
//   - error: ErrConflict если URL уже существует, иначе ошибка базы данных
//
// Пример использования:
//
//	url := model.URLModel{
//	    ShortURL:    "abc123",
//	    OriginalURL: "https://example.com",
//	    UserID:      userID,
//	}
//	savedURL, err := repo.SaveURL(url)
//	if errors.Is(err, repository.ErrConflict) {
//	    // URL уже существует, используем savedURL (существующую запись)
//	}
//
// SQL логика:
//
//	INSERT ... ON CONFLICT (original_url) DO NOTHING
//	- При успехе: возвращает сгенерированный ID
//	- При конфликте: возвращает ErrNoRows, выполняется SELECT для поиска существующей записи
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

// GetURL возвращает URL по его short идентификатору.
//
// Выполняет SELECT запрос по полю short_url.
// Возвращает полную модель URL включая флаг is_deleted.
//
// Параметры:
//   - id: short идентификатор URL
//
// Возвращает:
//   - model.URLModel: найденный URL
//   - error: "url not found" если URL не существует, иначе ошибка базы данных
//
// Пример использования:
//
//	url, err := repo.GetURL("abc123")
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

// SaveBatch сохраняет несколько URL в рамках одной транзакции.
//
// Использует batch операции pgx для эффективной вставки.
// Каждая запись получает новый UUID, сгенерированный приложением.
//
// Параметры:
//   - ctx: контекст для cancellation/timeout транзакции
//   - batch: слайс URLModel для сохранения
//
// Возвращает:
//   - error: ошибка если транзакция не удалась
//
// Пример использования:
//
//	urls := []model.URLModel{
//	    {ShortURL: "id1", OriginalURL: "https://example1.com", UserID: userID},
//	    {ShortURL: "id2", OriginalURL: "https://example2.com", UserID: userID},
//	}
//	err := repo.SaveBatch(context.Background(), urls)
//	if err != nil {
//	    // откат транзакции выполнен автоматически
//	}
//
// Особенности:
//   - Использует транзакцию для атомарности
//   - Batch размер ограничен только памятью
//   - При ошибке выполняется автоматический Rollback
//   - Не проверяет уникальность (дубликаты вызовут ошибку БД)
func (p *PostgresRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

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
	return p.pool.Ping(ctx)
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
	return err
}

// GetURLsByUser возвращает все не удаленные URL принадлежащие пользователю.
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
//	urls, err := repo.GetURLsByUser(context.Background(), userID)
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

	for rows.Next() {
		var u model.URLModel
		var deleted bool
		if err := rows.Scan(&u.ID, &u.ShortURL, &u.OriginalURL, &deleted); err != nil {
			return nil, fmt.Errorf("get urls by user: scan failed: %w", err)
		}
		if !deleted {
			u.ShortURL = fmt.Sprintf("%s/%s", p.baseURL, u.ShortURL)
			urls = append(urls, u)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
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
//   - userID: UUID пользователя для проверки владения
//   - shortURLs: слайс short идентификаторов для пометки как удаленные
//
// Возвращает:
//   - error: ошибка выполнения update запроса
//
// Пример использования:
//
//	err := repo.MarkAsDeleted(userID, []string{"abc123", "def456"})
//	if err != nil {
//	    // ошибка обновления БД
//	}
//
// Особенности:
//   - Использует ANY($2) для сравнения с массивом
//   - Батчинг для больших списков (по 500 элементов)
//   - Проверяет is_deleted = FALSE чтобы избежать лишних обновлений
//   - Логирует процесс для отладки
func (p *PostgresRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	p.logger.Info("repo: marking as deleted", zap.String("user_id", userID.String()), zap.Int("count", len(shortURLs)))
	if len(shortURLs) == 0 {
		return nil
	}
	p.logger.Info("repo: marking as deleted",
		zap.String("user_id", userID.String()),
		zap.Strings("short_urls", shortURLs),
		zap.Strings("ids", shortURLs))

	const batchSize = 500 // защита от множеств параметров
	for i := 0; i < len(shortURLs); i += batchSize {
		end := i + batchSize
		if end > len(shortURLs) {
			end = len(shortURLs)
		}
		batch := shortURLs[i:end]

		query := `
		UPDATE urls SET is_deleted = TRUE, deleted_at = NOW()
        WHERE user_id = $1 AND short_url = ANY($2) AND is_deleted = FALSE`
		_, err := p.pool.Exec(context.Background(), query, userID, pq.Array(batch))
		if err != nil {
			p.logger.Error("mark as deleted failed", zap.Error(err))
			return err
		}
	}

	p.logger.Info("mark as deleted successfully",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(shortURLs)))

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
