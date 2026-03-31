// Package repository предоставляет реализации хранилищ для URL shortener.
package repository

import (
	"context"
	"fmt"
	"github.com/avast/retry-go/v4"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

// MemoryRepository реализует хранилище URL в оперативной памяти (in-memory).
//
// Особенности:
//   - Хранение данных в map[string]URLModel
//   - Потокобезопасность через sync.Mutex
//   - Автоматическая генерация уникальных ID с retry-логикой при коллизиях
//   - Поддержка контекста для отмены операций (graceful shutdown, таймауты)
//   - Мягкое удаление (soft delete)
//   - Структурированное логирование через zap.Logger
//
// Архитектура:
//   - Данные: map[shortID]URLModel
//   - Синхронизация: Mutex для всех операций
//   - Генерация ID: idgen.IDGen() с retry при коллизиях (библиотека avast/retry-go)
//   - Логирование: zap.Logger для отладки и мониторинга
//
// Используется для:
//   - Unit и integration тестов
//   - Development окружения
//   - Демонстрационных целей
//   - Систем с малым объемом данных
//
// Ограничения:
//   - Данные теряются при перезапуске приложения
//   - Потребление памяти растет с количеством URL
//   - Не подходит для production с большим объемом данных
type MemoryRepository struct {
	mu      sync.RWMutex // несколько горутин смогут читать одновременно
	urls    map[string]model.URLModel
	baseURL string
	logger  *zap.Logger
}

// NewMemoryRepository создает новый in-memory репозиторий.
//
// Инициализирует пустую map для хранения URL и логгер для мониторинга.
// Не требует внешних зависимостей (файлов, БД).
//
// Параметры:
//   - baseURL: базовый URL для формирования полных коротких ссылок
//   - logger: структурированный логгер для записи событий репозитория
//
// Возвращает:
//   - *MemoryRepository: готовый к использованию репозиторий
//
// Пример использования:
//
//	logger, _ := zap.NewProduction()
//	repo := repository.NewMemoryRepository("http://localhost:8080", logger)
//
//	// Используется в тестах:
//	func TestService(t *testing.T) {
//	    repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
//	    svc := service.New(repo)
//	    // тестирование...
//	}
//
// Примечания:
//   - Данные существуют только во время работы приложения
//   - Нет persistence между запусками
//   - Идеален для тестов благодаря изоляции
func NewMemoryRepository(baseURL string, logger *zap.Logger) *MemoryRepository {
	return &MemoryRepository{
		urls:    make(map[string]model.URLModel),
		baseURL: baseURL,
		logger:  logger,
	}
}

// SaveURL сохраняет URL в memory хранилище с поддержкой отмены через контекст.
//
// Выполняет:
//  1. Проверку уникальности предоставленного UUID (если есть)
//  2. Генерацию нового UUID через idgen.IDGen() если не предоставлен
//  3. Retry логику с экспоненциальной задержкой при коллизиях сгенерированных ID
//  4. Сохранение в memory map
//  5. Структурированное логирование каждого этапа
//
// Параметры:
//   - ctx: контекст для отмены операции (таймаут, graceful shutdown)
//   - urlModel: URLModel для сохранения (UUID может быть пустым)
//
// Возвращает:
//   - *model.URLModel: указатель на сохранённую модель
//   - error: ошибка если UUID уже существует или не удалось сгенерировать уникальный ID
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
//	if err != nil {
//	    // обработка ошибки (коллизия, отмена контекста или ошибка генерации)
//	}
//
// Логика генерации ID:
//   - Если urlModel.UUID не пустой: проверка уникальности и сохранение
//   - Если пустой: генерация через idgen.IDGen() с использованием retry-пакета
//   - При коллизии: retry с экспоненциальной задержкой
//   - После 5 неудачных попыток: возврат ошибки
//   - При отмене контекста: немедленное прекращение попыток
//
// Особенности retry-логики:
//   - Context: поддержка отмены операции
//   - Attempts: 5 попыток генерации
//   - DelayType: экспоненциальная задержка (BackOffDelay)
//   - MaxDelay: максимальная задержка 150ms
//   - LastErrorOnly: возврат только последней ошибки
//   - OnRetry: логирование каждой неудачной попытки
func (m *MemoryRepository) SaveURL(ctx context.Context, urlModel model.URLModel) (*model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Защищаем map от одновременного доступа из разных горутин
	m.mu.Lock()
	defer m.mu.Unlock()

	// UUID уже передан извне
	if urlModel.UUID != "" {
		// Проверяем, не занят ли уже этот идентификатор
		if _, ok := m.urls[urlModel.UUID]; ok {
			m.logger.Warn("Attempt to save copy UUID",
				zap.String("uuid", urlModel.UUID),
				zap.String("original", urlModel.OriginalURL))

			return nil, fmt.Errorf("url with id %s already exists", urlModel.UUID)
		}
		// Всё ок — сохраняем копию модели в map
		m.urls[urlModel.UUID] = urlModel

		m.logger.Info("URL with unique UUID saved",
			zap.String("uuid", urlModel.UUID),
			zap.String("original", urlModel.OriginalURL))
		// Возвращаем указатель на копию сохранённой модели
		saved := urlModel
		return &saved, nil
	}

	// UUID нужно сгенерировать
	var saved model.URLModel // сюда запишем финальную успешную модель

	// retry.Do функция, которая повторяет переданное замыкание
	// до успеха или исчерпания попыток / отмены контекста
	err := retry.Do(
		// Это замыкание (анонимная функция)
		func() error {
			// Пробуем сгенерировать короткий идентификатор
			newID, genErr := idgen.IDGen()
			if genErr != nil {
				// Если генератор сломался, то это не коллизия,
				// а фатальная ошибка, стопаем retry
				return retry.Unrecoverable(genErr)
			}
			// Проверяем, свободен ли этот идентификатор в нашей map
			if _, exist := m.urls[newID]; exist {
				// Коллизия
				// Возвращаем обычную ошибку. retry.Do увидит и попробует ещё раз
				m.logger.Debug("Collision during gen short ID",
					zap.String("generated_id", newID))
				return fmt.Errorf("collision on generated id: %s", newID)
			}
			// Успех
			// Заполняем копию модели, которую получили на входе
			saved = urlModel
			// Сохраняем изменённую копию в хранилище
			saved.UUID = newID // map хранит свою копию структуры
			// Запоминаем успешный результат для возврата
			saved.ShortURL = newID // ещё одна копия, финальная версия

			// Сохраняем в map
			m.urls[newID] = saved

			return nil
		},

		// Если сервер начал graceful shutdown или запрос отменён по таймауту —
		// retry немедленно прекратит попытки и вернёт ctx.Err()
		retry.Context(ctx),

		// Максимальное количество попыток
		retry.Attempts(5),
		// Тип задержки между попытками
		retry.DelayType(retry.BackOffDelay),
		// Ограничиваем максимальную паузу
		// Чтобы даже при 5 попытках не ждать минуты
		retry.MaxDelay(150*time.Millisecond),
		// Возвращаем только последнюю реальную ошибку,
		retry.LastErrorOnly(true),

		// Логируем каждую неудачную попытку
		retry.OnRetry(func(n uint, err error) {
			// attempt начинается с 0, поэтому +1 для человека
			m.logger.Debug("Fail attempt to gen short ID",
				zap.Uint("attempt", n+1),
				zap.Error(err))
		}),
	)

	if err != nil {
		// Либо исчерпаны попытки, либо контекст отменён
		m.logger.Error("Fail to gen unique short ID after attempts",
			zap.String("original_url", urlModel.OriginalURL),
			zap.Error(err))
		// Возвращаем пустую модель + ошибку с обёрткой
		return nil, fmt.Errorf("failed to generate unique short ID after retries: %w", err)
	}

	// Успех
	// savedModel содержит полностью заполненную модель
	m.logger.Info("Short URL created and saved",
		zap.String("short_id", saved.UUID),
		zap.String("original_url", saved.OriginalURL))

	// Возвращаем финальную версию
	return &saved, nil
}

// GetURL возвращает URL по его идентификатору.
//
// Ищет URL в memory map по shortID/UUID.
// Не проверяет флаг IsDeleted - эта проверка выполняется на уровне сервиса.
//
// Параметры:
//   - id: shortID или UUID URL
//
// Возвращает:
//   - *model.URLModel: указатель на найденный URL (копия для безопасности)
//   - error: ошибка "url not found" если URL не существует
//
// Пример использования:
//
//	url, err := repo.GetURL("abc123")
//	if err != nil {
//	    // URL не найден
//	}
//	fmt.Println(url.OriginalURL)
//
// Примечания:
//   - Поиск чувствителен к регистру
//   - Не различает shortID и UUID (используются одинаковые значения)
//   - Возвращает копию структуры (безопасно для конкурентного доступа)
//   - Операция защищена мьютексом
func (m *MemoryRepository) GetURL(ctx context.Context, id string) (*model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	url, exists := m.urls[id]
	if !exists {
		m.logger.Debug("URL not found", zap.String("short_id", id))
		return nil, fmt.Errorf("url not found")
	}

	// Создаём копию и возвращаем указатель на неё — безопасно и экономно
	copy := url
	return &copy, nil
}

// SaveBatch сохраняет несколько URL одной атомарной операцией.
//
// Принимает слайс URLModel, у которых должны быть уже сгенерированы UUID.
// Пропускает элементы с пустым UUID с предупреждением в логах.
//
// Параметры:
//   - ctx: контекст для cancellation/timeout
//   - batch: слайс URLModel для сохранения
//
// Возвращает:
//   - error: nil если операция выполнена (даже если некоторые элементы пропущены)
//
// Пример использования:
//
//	urls := []model.URLModel{
//	    {UUID: "id1", OriginalURL: "https://example1.com", UserID: userID},
//	    {UUID: "id2", OriginalURL: "https://example2.com", UserID: userID},
//	}
//	err := repo.SaveBatch(context.Background(), urls)
//
// Примечания:
//   - Все URLModel должны иметь заполненное поле UUID
//   - Элементы без UUID игнорируются с предупреждением в логах
//   - При дубликатах UUID последний перезаписывает предыдущий
//   - Операция атомарна благодаря единому lock
//   - Логирует количество сохраненных элементов
func (m *MemoryRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	savedCount := 0
	for _, item := range batch {
		if item.UUID == "" {
			m.logger.Warn("Batch element has been stolen: Has no UUID",
				zap.String("original", item.OriginalURL))
			continue
		}
		m.urls[item.UUID] = item
		savedCount++
	}

	if savedCount == 0 {
		m.logger.Info("Batch element has been stolen: Nothing to save")
	}
	if savedCount > 0 {
		m.logger.Info("Batch element has been saved")
	}
	return nil
}

// Clear полностью очищает хранилище.
//
// Удаляет все URL из memory map и создает новую пустую map.
// Полезно для сброса состояния между тестами.
//
// Возвращает:
//   - error: всегда nil
//
// Пример использования в тестах:
//
//	func TestSomething(t *testing.T) {
//	    repo := repository.NewMemoryRepository("http://localhost:8080", zap.NewNop())
//	    defer repo.Clear()
//	    // тестовая логика...
//	}
//
// Примечания:
//   - Данные безвозвратно удаляются
//   - Старая map становится доступной для garbage collection
//   - Логирует количество удаленных элементов
func (m *MemoryRepository) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldSize := len(m.urls)
	m.urls = make(map[string]model.URLModel)

	m.logger.Info("Repo has been cleared",
		zap.Int("has elements", oldSize),
		zap.Int("elements now", 0))

	return nil
}

// Ping проверяет доступность хранилища.
//
// В MemoryRepository всегда возвращает nil (хранилище всегда доступно).
// Метод существует для совместимости с интерфейсом Repository.
//
// Параметры:
//   - ctx: контекст (не используется)
//
// Возвращает:
//   - error: всегда nil
//
// Используется для health checks:
//
//	if err := repo.Ping(context.Background()); err != nil {
//	    // логика при недоступности хранилища
//	}
func (m *MemoryRepository) Ping(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}

// GetURLsByUser возвращает все URL принадлежащие указанному пользователю.
//
// Фильтрует URL по UserID и флагу IsDeleted, преобразует shortURL
// в полные URL путем добавления baseURL из конфигурации.
//
// Параметры:
//   - ctx: контекст для cancellation/timeout
//   - userID: UUID пользователя
//
// Возвращает:
//   - []model.URLModel: слайс URL пользователя с полными shortURL
//   - error: всегда nil
//
// Пример использования:
//
//	urls, _ := repo.GetURLsByUser(context.Background(), userID)
//	for _, url := range urls {
//	    fmt.Printf("%s -> %s\n", url.ShortURL, url.OriginalURL)
//	}
//
// Примечания:
//   - Возвращает только не удаленные URL (IsDeleted = false)
//   - Для uuid.Nil возвращает пустой слайс
//   - ShortURL преобразуется в полный URL с использованием baseURL из конфигурации
//   - Сортировка не гарантируется (порядок может меняться)
func (m *MemoryRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if userID == uuid.Nil {
		return []model.URLModel{}, nil
	}

	var result []model.URLModel

	for _, url := range m.urls {
		if url.UserID == userID && !url.IsDeleted {
			result = append(result, model.URLModel{
				ShortURL:    fmt.Sprintf("%s/%s", m.baseURL, url.ShortURL),
				OriginalURL: url.OriginalURL,
			})
		}
	}
	return result, nil
}

// MarkAsDeleted помечает URL как удаленные (soft delete).
//
// Находит URL по shortID и проверяет владельца (UserID).
// Если URL принадлежит указанному пользователю, устанавливает IsDeleted = true.
// Повторные вызовы для уже удаленных URL игнорируются.
//
// Параметры:
//   - userID: UUID пользователя для проверки владения
//   - shortURLs: слайс short идентификаторов для пометки как удаленные
//
// Возвращает:
//   - error: всегда nil
//
// Пример использования:
//
//	err := repo.MarkAsDeleted(userID, []string{"abc123", "def456"})
//	// Теперь GetURL("abc123") вернет URL с IsDeleted = true
//	// GetURLsByUser не будет включать эти URL
//
// Примечания:
//   - Удаляет только URL принадлежащие указанному userID
//   - Операция идемпотентна (повторные вызовы для удаленных URL не меняют состояние)
//   - Не удаляет данные из map, только меняет флаг
//   - Логирует количество фактически удаленных URL
func (m *MemoryRepository) MarkAsDeleted(ctx context.Context, userID uuid.UUID, shortURLs []string) error {

	if ctx.Err() != nil {
		return ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	updated := 0
	for _, shortURL := range shortURLs {
		if model, ok := m.urls[shortURL]; ok && model.UserID == userID && !model.IsDeleted {
			model.IsDeleted = true
			m.urls[shortURL] = model
			updated++
		}
	}
	if updated > 0 {
		m.logger.Info("URL marked as deleted",
			zap.String("user_id", userID.String()),
			zap.Int("modified", updated),
			zap.Int("requested", len(shortURLs)))
	}

	return nil
}

// Stats возвращает статистику сервиса из in-memory хранилища.
//
// Реализация:
//   - Захватывает полную блокировку mu.Lock() для обеспечения consistency.
//   - Подсчёт urls = len(m.urls) — O(1).
//   - Подсчёт уникальных пользователей выполняется за O(N) путём прохода по всем URL
//     и использования map[uuid.UUID]struct{}.
//   - Записи с userID == uuid.Nil логируются как Warn, но не влияют на результат подсчёта.
//
// Важно:
//   - При большом количестве URL (десятки и сотни тысяч) метод может быть относительно медленным
//     и удерживать блокировку на всё время подсчёта.
//   - Подходит только для разработки, тестов и небольших инстансов.
func (m *MemoryRepository) Stats() (urls int, users int) {

	// Захватываем блокировку
	m.mu.RLock() // только чтение
	defer m.mu.RUnlock()

	// Подсчитываем urls
	totalURLs := len(m.urls)
	// Создаём map для уникальных пользователей
	uniqueUsers := make(map[uuid.UUID]struct{})

	// Проходим по всем URL и собираем уникальные UserID
	for _, url := range m.urls {
		if url.UserID != uuid.Nil {
			uniqueUsers[url.UserID] = struct{}{} // добавляем
		} else {
			// только логируем предупреждение, но продолжаем подсчёт
			m.logger.Warn("Found URL with empty user ID", zap.String("short_url", url.ShortURL))

		}
	}

	// Считаем количество уникальных пользователей
	urls = totalURLs
	users = len(uniqueUsers)

	// Возвращаем результат
	return
}
