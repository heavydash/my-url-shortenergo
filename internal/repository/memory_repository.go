// Package repository предоставляет реализации хранилищ для URL shortener.
package repository

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"

	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

// MemoryRepository реализует хранилище URL в оперативной памяти (in-memory).
//
// Особенности:
//   - Хранение данных в map[string]URLModel
//   - Потокобезопасность через sync.Mutex
//   - Автоматическая генерация уникальных ID
//   - Retry логика при коллизиях ID
//   - Мягкое удаление (soft delete)
//
// Архитектура:
//   - Данные: map[shortID]URLModel
//   - Синхронизация: Mutex для всех операций
//   - Генерация ID: idgen.IDGen() с retry при коллизиях
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
	mu   sync.Mutex
	urls map[string]model.URLModel
}

// NewMemoryRepository создает новый in-memory репозиторий.
//
// Инициализирует пустую map для хранения URL.
// Не требует внешних зависимостей (файлов, БД).
//
// Возвращает:
//   - *MemoryRepository: готовый к использованию репозиторий
//
// Пример использования:
//
//	repo := repository.NewMemoryRepository()
//	// Используется в тестах:
//	func TestService(t *testing.T) {
//	    repo := repository.NewMemoryRepository()
//	    svc := service.New(repo)
//	    // тестирование...
//	}
//
// Примечания:
//   - Данные существуют только во время работы приложения
//   - Нет persistence между запусками
//   - Идеален для тестов благодаря изоляции
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		urls: make(map[string]model.URLModel),
	}
}

// SaveURL сохраняет URL в memory хранилище.
//
// Выполняет:
//  1. Проверку уникальности предоставленного UUID (если есть)
//  2. Генерацию нового UUID через idgen.IDGen() если не предоставлен
//  3. Retry логику (до 5 попыток) при коллизиях сгенерированных ID
//  4. Сохранение в memory map
//
// Параметры:
//   - model: URLModel для сохранения (UUID может быть пустым)
//
// Возвращает:
//   - model.URLModel: сохраненная модель с заполненными полями
//   - error: ошибка если UUID уже существует или не удалось сгенерировать уникальный ID
//
// Пример использования:
//
//	url := model.URLModel{
//	    OriginalURL: "https://example.com",
//	    UserID:      userID,
//	}
//	savedURL, err := repo.SaveURL(url)
//	if err != nil {
//	    // обработка ошибки (коллизия или ошибка генерации)
//	}
//
// Логика генерации ID:
//   - Если URLModel.UUID не пустой: проверка уникальности и сохранение
//   - Если пустой: генерация через idgen.IDGen() с 5 попытками
//   - При коллизии: следующая попытка с новым ID
//   - После 5 неудачных попыток: возврат ошибки
func (m *MemoryRepository) SaveURL(model model.URLModel) (model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newModel := model
	if newModel.UUID != "" {
		if _, ok := m.urls[newModel.UUID]; ok {
			return model, fmt.Errorf("url with id %s already exists", newModel.UUID)
		}
		m.urls[newModel.UUID] = newModel
		return newModel, nil
	}
	maxAttempts := 5
	log.Printf("Attempting to save URL, current Urls size: %d", len(m.urls))
	for attempt := 0; attempt < maxAttempts; attempt++ {
		newID, err := idgen.IDGen()
		if err != nil {
			return model, fmt.Errorf("error generating uuid: %w", err)
		}
		currentModel := model
		log.Printf("Attempt %d, checking ID: %s, Urls size: %d", attempt, newID, len(m.urls))
		if _, ok := m.urls[newID]; !ok {
			currentModel.UUID = newID
			m.urls[newID] = currentModel
			return currentModel, nil
		}
	}
	return model, fmt.Errorf("failed to generate unique short ID after %d attempts", maxAttempts)
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
//   - model.URLModel: найденный URL
//   - error: ошибка "not found" если URL не существует
//
// Пример использования:
//
//	url, err := repo.GetURL("abc123")
//	if err != nil {
//	    // URL не найден
//	}
//
// Примечания:
//   - Поиск чувствителен к регистру
//   - Не различает shortID и UUID (используются одинаковые значения)
//   - Операция защищена мьютексом для конкурентного доступа
func (m *MemoryRepository) GetURL(id string) (model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.urls[id]; ok {
		return m.urls[id], nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

// SaveBatch сохраняет несколько URL одной атомарной операцией.
//
// Принимает слайс URLModel, у которых должны быть уже сгенерированы UUID.
// Не выполняет проверку уникальности - предполагается что UUID уникальны.
//
// Параметры:
//   - ctx: контекст для cancellation/timeout (не используется в текущей реализации)
//   - batch: слайс URLModel для сохранения
//
// Возвращает:
//   - error: всегда nil в текущей реализации
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
//   - При дубликатах UUID последний перезаписывает предыдущий
//   - Операция атомарна благодаря единому lock
func (m *MemoryRepository) SaveBatch(сtx context.Context, batch []model.URLModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range batch {
		m.urls[item.UUID] = item
	}
	return nil
}

// Clear полностью очищает хранилище.
//
// Удаляет все URL из memory map и создает новую пустую map.
// Полезно для сброса состояния между тестами.
//
// Возвращает:
//   - error: всегда nil в текущей реализации
//
// Пример использования в тестах:
//
//	func TestSomething(t *testing.T) {
//	    repo := repository.NewMemoryRepository()
//	    defer repo.Clear() // очистка после теста
//	    // тестовая логика...
//	}
//
// Примечания:
//   - Данные безвозвратно удаляются
//   - Старая map становится доступной для garbage collection
//   - Логирует новый размер хранилища (0)
func (m *MemoryRepository) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls = make(map[string]model.URLModel)
	log.Printf("Cleared Urls, new size: %d", len(m.urls))
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
// в полные URL путем добавления baseURL (http://localhost:8080).
//
// Параметры:
//   - ctx: контекст для cancellation/timeout
//   - userID: UUID пользователя
//
// Возвращает:
//   - []model.URLModel: слайс URL пользователя с полными shortURL
//   - error: всегда nil в текущей реализации
//
// Пример использования:
//
//	urls, _ := repo.GetURLsByUser(context.Background(), userID)
//	for _, url := range urls {
//	    // url.ShortURL: "http://localhost:8080/abc123"
//	    // url.OriginalURL: "https://example.com"
//	}
//
// Примечания:
//   - Возвращает только не удаленные URL (IsDeleted = false)
//   - Для uuid.Nil возвращает пустой слайс
//   - Преобразование shortURL добавляет хардкодный baseURL
func (m *MemoryRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if userID == uuid.Nil {
		return []model.URLModel{}, nil
	}

	var result []model.URLModel
	baseURL := "http://localhost:8080"

	for _, m := range m.urls {
		if m.UserID == userID && !m.IsDeleted {
			result = append(result, model.URLModel{
				ShortURL:    fmt.Sprintf("%s/%s", baseURL, m.ShortURL),
				OriginalURL: m.OriginalURL,
			})
		}
	}
	return result, nil
}

// MarkAsDeleted помечает URL как удаленные (soft delete).
//
// Находит URL по shortID и проверяет владельца (UserID).
// Если URL принадлежит указанному пользователю, устанавливает IsDeleted = true.
//
// Параметры:
//   - userID: UUID пользователя для проверки владения
//   - shortURLs: слайс short идентификаторов для пометки как удаленные
//
// Возвращает:
//   - error: всегда nil в текущей реализации
//
// Пример использования:
//
//	err := repo.MarkAsDeleted(userID, []string{"abc123", "def456"})
//	// Теперь GetURL("abc123") вернет URL с IsDeleted = true
//	// GetURLsByUser не будет включать эти URL
//
// Примечания:
//   - Удаляет только URL принадлежащие указанному userID
//   - Операция идемпотентна (повторные вызовы не меняют состояние)
//   - Не удаляет данные из map, только меняет флаг
func (m *MemoryRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, shortURL := range shortURLs {
		if model, ok := m.urls[shortURL]; ok && model.UserID == userID {
			model.IsDeleted = true
			m.urls[shortURL] = model
		}
	}
	return nil
}
