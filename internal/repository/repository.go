// Package repository предоставляет интерфейсы и реализации для хранения данных URL.
// Поддерживает несколько бэкендов: память, файл, PostgreSQL.
//
// Основные реализации:
//
//	MemoryRepository   - хранение в памяти (для тестов и разработки)
//	FileRepository     - хранение в JSON файле (персистентное хранение)
//	PostgresRepository - хранение в PostgreSQL (промышленное использование)
//
// Все реализации удовлетворяют интерфейсу URLRepository.
// Для тестирования генерируются моки с помощью go:generate.
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

//go:generate mockgen -source=$GOFILE -destination=../repository/mocks/mock_repository.go -package=mocks

// URLRepository определяет контракт для работы с хранилищем URL.
// Используется handler'ами для абстракции над конкретным бэкендом хранения.
//
// Все методы должны быть потокобезопасными при использовании в конкурентной среде.
type URLRepository interface {
	// SaveURL сохраняет URL в хранилище.
	//
	// Параметры:
	//   m - модель URL для сохранения. Поле UUID может быть пустым для автоматической генерации.
	//
	// Возвращает:
	//   model.URLModel - сохраненная запись с заполненными полями UUID и ShortURL
	//   error - ошибка если URL уже существует или возникла проблема при сохранении
	//
	// Пример:
	//   url := model.URLModel{OriginalURL: "https://example.com"}
	//   saved, err := repo.SaveURL(url)
	SaveURL(m model.URLModel) (model.URLModel, error)

	// GetURL возвращает URL по его короткому идентификатору.
	//
	// Параметры:
	//   id - короткий идентификатор URL (UUID или сгенерированная строка)
	//
	// Возвращает:
	//   model.URLModel - найденная запись URL
	//   error - ошибка если URL не найден или был удален
	//
	// Пример:
	//   url, err := repo.GetURL("abc123")
	GetURL(id string) (model.URLModel, error)

	// SaveBatch сохраняет несколько URL за одну операцию.
	// Используется для эффективного пакетного создания коротких URL.
	//
	// Параметры:
	//   ctx   - контекст для отмены операции и таймаутов
	//   batch - список моделей URL для сохранения
	//
	// Возвращает:
	//   error - ошибка если хотя бы один URL не удалось сохранить
	//
	// Примечание:
	//   - Реализации должны обеспечивать атомарность операции (все или ничего)
	//   - При ошибке желательно выполнять откат всех изменений
	SaveBatch(ctx context.Context, batch []model.URLModel) error

	// Clear удаляет все данные из хранилища.
	// Используется в основном для тестирования.
	//
	// Возвращает:
	//   error - ошибка при очистке хранилища
	//
	// Внимание:
	//   - В production коде этот метод не должен использоваться
	//   - FileRepository и PostgresRepository могут требовать особой обработки
	Clear() error

	// Ping проверяет доступность хранилища.
	// Используется для health checks эндпоинта /ping.
	//
	// Параметры:
	//   ctx - контекст для таймаута проверки
	//
	// Возвращает:
	//   error - ошибка если хранилище недоступно
	//
	// Пример:
	//   err := repo.Ping(context.Background())
	Ping(ctx context.Context) error

	// GetURLsByUser возвращает все URL, созданные указанным пользователем.
	// Используется для эндпоинта /api/user/urls.
	//
	// Параметры:
	//   ctx    - контекст для отмены операции
	//   userID - UUID пользователя
	//
	// Возвращает:
	//   []model.URLModel - список URL пользователя, может быть пустым
	//   error - ошибка при получении данных
	//
	// Пример:
	//   urls, err := repo.GetURLsByUser(ctx, userID)
	GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error)

	// MarkAsDeleted помечает указанные URL как удаленные (soft delete).
	// URL остаются в хранилище с флагом IsDeleted = true.
	// Используется для асинхронного удаления через URLDeleter.
	//
	// Параметры:
	//   userID    - UUID пользователя, который удаляет URL
	//   shortURLs - список коротких идентификаторов URL для удаления
	//
	// Возвращает:
	//   error - ошибка если URL не найден или не принадлежит пользователю
	//
	// Примечание:
	//   - Метод должен проверять принадлежность URL пользователю
	//   - Удаление только своих URL (без прав на удаление чужих)
	MarkAsDeleted(userID uuid.UUID, shortURLs []string) error
}
