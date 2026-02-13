// Package repository предоставляет реализации хранилищ (репозиториев) для URL shortener.
// Репозитории абстрагируют доступ к данным и реализуют паттерн "Repository".
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/google/uuid"

	"github.com/heavydash/my-url-shortenergo/internal/idgen"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

// FileRepository реализует хранилище URL в файле формата JSON-lines.
//
// Особенности:
//   - Хранение данных в формате JSON, каждая запись на новой строке
//   - In-memory кэш для быстрого доступа (map)
//   - Потокобезопасность через sync.RWMutex
//   - Автоматическая загрузка данных при инициализации
//   - Атомарная запись через append-only файл
//
// Архитектура:
//   - Файл: append-only JSON-lines (каждая строка - JSON объекта URL)
//   - Память: map[string]URLModel для быстрого поиска по ID
//   - Синхронизация: RWMutex для конкурентного доступа
//
// Пример содержимого файла:
//
//	{"uuid":"abc123","short_url":"abc123","original_url":"https://example.com"}\n
//	{"uuid":"def456","short_url":"def456","original_url":"https://google.com"}\n
//
// Используется для:
//   - Development окружения
//   - Single-instance деплоев
//   - Резервного хранилища
//   - Тестов и прототипирования
type FileRepository struct {
	mu      sync.RWMutex
	file    *os.File
	encoder *json.Encoder
	urls    map[string]model.URLModel
	baseURL string
}

// NewFileRepository создает новый FileRepository и загружает данные из файла.
//
// Инициализирует:
//  1. Открытие файла в режиме чтение/запись с созданием если не существует
//  2. Создание JSON энкодера для записи
//  3. In-memory map для кэширования
//  4. Загрузку существующих данных из файла
//
// Параметры:
//   - path: путь к файлу хранилища (например, "/data/urls.json")
//   - baseURL: базовый URL для формирования коротких ссылок (например, "http://localhost:8080")
//
// Возвращает:
//   - *FileRepository: готовый к использованию репозиторий
//
// Паникует если:
//   - Не удалось открыть/создать файл
//   - Нет прав на запись в указанный путь
//
// Пример использования:
//
//	repo := repository.NewFileRepository("/tmp/urls.json")
//	defer repo.file.Close()
//
// Примечания:
//   - Файл создается с правами 0644 (rw-r--r--)
//   - Режим O_APPEND гарантирует атомарную запись
//   - Рекомендуется использовать абсолютные пути
func NewFileRepository(path string, baseURL string) *FileRepository {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	repo := &FileRepository{
		file:    file,
		encoder: json.NewEncoder(file),
		urls:    make(map[string]model.URLModel),
		baseURL: baseURL,
	}
	repo.loadFromFile()
	return repo
}

// loadFromFile загружает данные из файла в memory cache.
//
// Читает файл с начала и десериализует каждую строку как JSON объект URLModel.
// Некорректные строки игнорируются (продолжается чтение следующей строки).
//
// Вызывается автоматически при инициализации репозитория.
//
// Примечания:
//   - Используется json.Decoder для потокового чтения
//   - Игнорирует синтаксические ошибки в отдельных строках
//   - Сбрасывает позицию файла на начало (Seek(0, 0))
func (r *FileRepository) loadFromFile() {
	r.file.Seek(0, 0)
	dec := json.NewDecoder(r.file)
	for {
		var u model.URLModel
		if err := dec.Decode(&u); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		if u.UUID != "" {
			r.urls[u.UUID] = u
		}
	}
}

// SaveURL сохраняет URL в файловое хранилище.
//
// Выполняет:
//  1. Генерацию UUID если не предоставлен
//  2. Проверку уникальности UUID
//  3. Сериализацию в JSON и запись в файл
//  4. Добавление перевода строки
//  5. Кэширование в memory map
//
// Параметры:
//   - url: URLModel для сохранения (UUID может быть пустым)
//
// Возвращает:
//   - model.URLModel: сохраненная модель с заполненными полями
//   - error: ошибка если UUID уже существует или ошибка записи
//
// Пример использования:
//
//	url := model.URLModel{
//	    OriginalURL: "https://example.com",
//	    UserID:      userID,
//	}
//	savedURL, err := repo.SaveURL(url)
//	// savedURL.UUID и savedURL.ShortURL теперь заполнены
//
// Примечания:
//   - UUID используется и как идентификатор и как short_url
//   - Операция атомарна благодаря режиму O_APPEND
//   - Конкурентные вызовы защищены мьютексом
func (r *FileRepository) SaveURL(url model.URLModel) (model.URLModel, error) {
	r.mu.Lock()
	// Защита от race condition при параллельном доступе
	defer r.mu.Unlock()
	if url.UUID == "" {
		id, err := idgen.IDGen()
		if err != nil {
			return url, err
		}
		url.UUID = id
		url.ShortURL = id
	}
	if _, ok := r.urls[url.UUID]; ok {
		return url, fmt.Errorf("id already exists")
	}
	if err := r.encoder.Encode(url); err != nil {
		return url, err
	}
	if _, err := r.file.Write([]byte("\n")); err != nil {
		return url, err
	}
	r.urls[url.UUID] = url
	return url, nil
}

// GetURL возвращает URL по его идентификатору.
//
// Ищет URL в memory cache по UUID. Если не найден, возвращает ошибку.
// Не выполняет чтение из файла - вся информация загружена в память.
//
// Параметры:
//   - id: UUID или short_url URL (в данной реализации они одинаковы)
//
// Возвращает:
//   - model.URLModel: найденный URL
//   - error: ошибка "not found" если URL не существует
//
// Пример использования:
//
//	url, err := repo.GetURL("abc123")
//	if err != nil {
//	    // обработка "not found"
//	}
//	redirect := url.OriginalURL
//
// Примечания:
//   - Поиск только по UUID (short_url должен совпадать с UUID)
//   - Не проверяет флаг IsDeleted (проверка на уровне сервиса)
//   - Чтение защищено RLock для конкурентного доступа
func (r *FileRepository) GetURL(id string) (model.URLModel, error) {
	r.mu.RLock()
	// Защита от race condition при параллельном доступе
	defer r.mu.RUnlock()
	if url, ok := r.urls[id]; ok {
		return url, nil
	}
	return model.URLModel{}, fmt.Errorf("not found")
}

// SaveBatch сохраняет несколько URL одной атомарной операцией.
//
// Используется для пакетного сохранения URL из batch запросов.
// Все URL сохраняются в рамках одной транзакции (под одним lock).
//
// Параметры:
//   - ctx: контекст для cancellation/timeout
//   - batch: слайс URLModel для сохранения
//
// Возвращает:
//   - error: ошибка если не удалось сохранить любой из URL
//
// Пример использования:
//
//	urls := []model.URLModel{url1, url2, url3}
//	err := repo.SaveBatch(context.Background(), urls)
//	// либо все URL сохранены, либо ни одного
//
// Примечания:
//   - Все URL должны иметь заранее сгенерированные UUID
//   - Не проверяет уникальность UUID (должны быть уникальны)
//   - При ошибке часть URL может быть уже записана (нет rollback)
func (r *FileRepository) SaveBatch(ctx context.Context, batch []model.URLModel) error {
	r.mu.Lock()
	// Защита от race condition при параллельном доступе
	defer r.mu.Unlock()

	for _, m := range batch {
		if err := r.encoder.Encode(m); err != nil {
			return err
		}
		if _, err := r.file.Write([]byte("\n")); err != nil {
			return err
		}
		r.urls[m.UUID] = m
	}
	return nil
}

// Clear полностью очищает хранилище.
//
// Удаляет все данные:
//  1. Очищает файл (truncate to 0)
//  2. Сбрасывает позицию файла
//  3. Очищает memory cache
//
// Возвращает:
//   - error: ошибка операций с файлом
//
// Используется для:
//   - Тестов (setup/teardown)
//   - Аварийного восстановления
//   - Полного сброса данных
//
// Предупреждение:
//   - Операция необратима
//   - Нет backup автоматически
//   - Производственное использование не рекомендуется
func (r *FileRepository) Clear() error {
	r.mu.Lock()
	// Защита от race condition при параллельном доступе
	defer r.mu.Unlock()
	if err := r.file.Truncate(0); err != nil {
		return err
	}
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r.urls = make(map[string]model.URLModel)
	return nil
}

// Ping проверяет доступность хранилища.
//
// В реализации FileRepository всегда возвращает nil.
// Метод существует для совместимости с интерфейсом Repository.
//
// Параметры:
//   - ctx: контекст (не используется)
//
// Возвращает:
//   - error: всегда nil
//
// Пример использования:
//
//	if err := repo.Ping(context.Background()); err != nil {
//	    log.Println("Storage unavailable")
//	}
func (r *FileRepository) Ping(ctx context.Context) error {
	r.mu.Lock()
	// Защита от race condition при параллельном доступе
	defer r.mu.Unlock()
	return nil
}

// GetURLsByUser возвращает все URL принадлежащие указанному пользователю.
//
// Ищет URL в memory cache по UserID и возвращает их с преобразованными
// short_url в полные URL (с baseURL).
//
// Параметры:
//   - ctx: контекст для cancellation/timeout
//   - userID: UUID пользователя
//
// Возвращает:
//   - []model.URLModel: слайс URL пользователя
//   - error: всегда nil в текущей реализации
//
// Пример использования:
//
//	urls, err := repo.GetURLsByUser(context.Background(), userID)
//	for _, url := range urls {
//	    fmt.Printf("%s -> %s\n", url.ShortURL, url.OriginalURL)
//	}
//
// Примечания:
//   - Возвращает только не удаленные URL (IsDeleted = false)
//   - ShortURL преобразуется в полный URL с localhost:8080
//   - Для пустого userID возвращает пустой слайс
func (r *FileRepository) GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if userID == uuid.Nil {
		return []model.URLModel{}, nil
	}

	var result []model.URLModel

	for _, url := range r.urls {
		if url.UserID == userID && !url.IsDeleted {
			result = append(result, model.URLModel{
				ShortURL:    fmt.Sprintf("%s/%s", r.baseURL, url.ShortURL),
				OriginalURL: url.OriginalURL,
			})
		}
	}
	return result, nil
}

// MarkAsDeleted помечает URL как удаленные (soft delete).
//
// Находит URL по shortURL и userID, устанавливает флаг IsDeleted = true.
// URL остается в хранилище, но не возвращается в GetURLsByUser.
//
// Параметры:
//   - userID: UUID пользователя (для проверки владения)
//   - shortURLs: слайс short идентификаторов для удаления
//
// Возвращает:
//   - error: всегда nil в текущей реализации
//
// Пример использования:
//
//	err := repo.MarkAsDeleted(userID, []string{"abc123", "def456"})
//	// URL помечены как удаленные
//
// Примечания:
//   - Удаляет только URL принадлежащие указанному userID
//   - Не удаляет данные физически (soft delete)
//   - Изменения сохраняются только в memory cache (не в файл)
func (r *FileRepository) MarkAsDeleted(userID uuid.UUID, shortURLs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, short := range shortURLs {
		if model, ok := r.urls[short]; ok && model.UserID == userID {
			model.IsDeleted = true
			r.urls[short] = model
		}
	}

	return nil
}
