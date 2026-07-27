package repository

import (
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"os"
	"testing"
)

// Package repository содержит тесты для файловой реализации хранилища URL.
//
// Тесты проверяют корректную работу FileRepository с JSON-файлом,
// включая сохранение, поиск, пакетное сохранение, получение по пользователю,
// мягкое удаление и сбор статистики.
//
// Каждый тест создаёт временный файл и удаляет его после выполнения через t.Cleanup().
//
// Особенности FileRepository:
//   - Данные сохраняются в JSON-файл (append-only)
//   - При дубликате UUID сейчас возвращается ошибка "id already exists"
//   - В отличие от MemoryRepository здесь проверка дубликата работает
func TestFileRepository_SaveURL(t *testing.T) {
	const testFile = "test_file_repository.json"

	// Создаём файловый репозиторий для теста
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения теста
	t.Cleanup(func() {
		_ = os.Remove(testFile) // очищаем файл после теста
	})

	tests := []struct {
		nameTest    string
		input       model.URLModel
		wantErr     bool
		description string
	}{
		{
			nameTest: "Save new URL",
			input: model.URLModel{
				OriginalURL: "https://example.com",
				UserID:      uuid.New(),
			},
			wantErr:     false,
			description: "The new URL must be saved to the file successfully",
		},
		{
			nameTest: "Save URL with provided UUID",
			input: model.URLModel{
				UUID:        "file-uuid-123",
				ShortURL:    "file-uuid-123",
				OriginalURL: "https://google.com",
				UserID:      uuid.New(),
			},
			wantErr:     false,
			description: "URL with an explicitly specified UUID",
		},
		{
			nameTest: "Save duplicate UUID",
			input: model.URLModel{
				UUID:        "file-duplicate",
				ShortURL:    "file-duplicate",
				OriginalURL: "https://duplicate.com",
			},
			wantErr:     true,
			description: "Duplicate UUID - FileRepository returns an error",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Подготавливаем первое сохранение для теста дубликата
			if tt.nameTest == "Save duplicate UUID" {
				first := model.URLModel{
					UUID:        "file-duplicate",
					ShortURL:    "file-duplicate",
					OriginalURL: "https://first.com",
				}
				_, err := repo.SaveURL(t.Context(), first)
				require.NoError(t, err, "Couldn't save the first instance")
			}

			// Выполняем сохранение URL
			saved, err := repo.SaveURL(t.Context(), tt.input)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), "already exists", tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.NotEmpty(t, saved.UUID)
			assert.Equal(t, tt.input.OriginalURL, saved.OriginalURL)
		})
	}
}

// TestFileRepository_GetURL тестирует получение URL по короткому идентификатору из файла.
func TestFileRepository_GetURL(t *testing.T) {
	const testFile = "test_file_get.json"

	// Создаём файловый репозиторий
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения
	t.Cleanup(func() { _ = os.Remove(testFile) })

	// Подготавливаем тестовые данные
	urlModel := model.URLModel{
		UUID:        "get-test-1",
		ShortURL:    "get-test-1",
		OriginalURL: "https://ya.ru",
		UserID:      uuid.New(),
	}
	_, _ = repo.SaveURL(t.Context(), urlModel)

	tests := []struct {
		nameTest    string
		shortID     string
		wantErr     bool
		description string
	}{
		{
			nameTest:    "Found existing URL",
			shortID:     "get-test-1",
			wantErr:     false,
			description: "The existing URL must be found",
		},
		{
			nameTest:    "Not found",
			shortID:     "non-existent",
			wantErr:     true,
			description: "A non-existent ID should return an error",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Выполняем поиск по идентификатору
			got, err := repo.GetURL(t.Context(), tt.shortID)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.shortID, got.UUID)
			assert.Equal(t, "https://ya.ru", got.OriginalURL)
		})
	}
}

// TestFileRepository_SaveBatch тестирует пакетное сохранение нескольких URL в файл.
func TestFileRepository_SaveBatch(t *testing.T) {
	const testFile = "test_file_batch.json"

	// Создаём файловый репозиторий
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения
	t.Cleanup(func() { _ = os.Remove(testFile) })

	// Подготавливаем батч
	batch := []model.URLModel{
		{OriginalURL: "https://github.com", UserID: uuid.New()},
	}

	// Выполняем пакетное сохранение
	err := repo.SaveBatch(t.Context(), batch)
	require.NoError(t, err)

	// Проверяем, что все URL успешно сохранены и доступны
	for _, item := range batch {
		got, err := repo.GetURL(t.Context(), item.UUID)
		require.NoError(t, err)
		assert.Equal(t, item.OriginalURL, got.OriginalURL)
	}
}

// TestFileRepository_GetURLsByUser тестирует получение всех URL конкретного пользователя из файла.
func TestFileRepository_GetURLsByUser(t *testing.T) {
	const testFile = "test_file_user.json"

	// Создаём файловый репозиторий
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения
	t.Cleanup(func() { _ = os.Remove(testFile) })

	userID := uuid.New()

	// Сохраняем URL двух разных пользователей
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://ya.ru", UserID: userID})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://google.com", UserID: userID})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://vk.com", UserID: uuid.New()}) // другой пользователь

	// Получаем список URL конкретного пользователя
	urls, err := repo.GetURLsByUser(t.Context(), userID)
	require.NoError(t, err)
	assert.Len(t, urls, 2, "There should be 2 user URLs returned")
}

// TestFileRepository_MarkAsDeleted тестирует мягкое удаление URL через файловое хранилище.
func TestFileRepository_MarkAsDeleted(t *testing.T) {
	const testFile = "test_file_delete.json"

	// Создаём файловый репозиторий
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения
	t.Cleanup(func() { _ = os.Remove(testFile) })

	userID := uuid.New()
	ids := []string{"del1", "del2"}

	// Сохраняем два URL для пользователя
	_, _ = repo.SaveURL(t.Context(), model.URLModel{UUID: "del1", ShortURL: "del1", OriginalURL: "https://1.com", UserID: userID})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{UUID: "del2", ShortURL: "del2", OriginalURL: "https://2.com", UserID: userID})

	// Выполняем мягкое удаление
	err := repo.MarkAsDeleted(t.Context(), userID, ids)
	require.NoError(t, err)

	// Проверяем, что URL помечены как удалённые
	for _, id := range ids {
		got, _ := repo.GetURL(t.Context(), id)
		assert.True(t, got.IsDeleted, "The URL must be marked as deleted")
	}
}

// TestFileRepository_Stats тестирует подсчёт статистики из файлового хранилища.
func TestFileRepository_Stats(t *testing.T) {
	const testFile = "test_file_stats.json"

	// Создаём файловый репозиторий
	repo := NewFileRepository(testFile, "http://localhost:8080", zap.NewNop())

	// Удаляем тестовый файл после завершения
	t.Cleanup(func() { _ = os.Remove(testFile) })

	// Добавляем два URL от разных пользователей
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://ya.ru", UserID: uuid.New()})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://google.com", UserID: uuid.New()})

	// Получаем статистику
	urlsCount, usersCount := repo.Stats()
	assert.Equal(t, 2, urlsCount)
	assert.Equal(t, 2, usersCount)
}
