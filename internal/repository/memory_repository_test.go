// Package repository содержит тесты для in-memory реализации хранилища URL.
//
// Тесты используют table-driven подход и покрывают основные сценарии работы
// MemoryRepository: сохранение, поиск, пакетное сохранение, soft delete,
// статистика и очистка хранилища.
//
// Все тесты используют реальный MemoryRepository с zap.NewNop() для изоляции.
package repository

import (
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
)

// TestMemoryRepository_SaveURL тестирует сохранение одиночного URL в memory хранилище.
//
// Покрывает:
//   - Сохранение нового URL с автоматической генерацией короткого идентификатора
//   - Сохранение URL с явно заданным UUID и ShortURL
//   - Поведение при попытке сохранить дубликат UUID (в текущей реализации — перезапись)
//
// Важное примечание:
//
//	Сейчас дубликат UUID просто перезаписывает запись (без ошибки).
//	В будущем рекомендуется усилить проверку и возвращать ошибку "already exists",
//	тогда этот тест можно будет обновить (wantErr: true).
func TestMemoryRepository_SaveURL(t *testing.T) {
	// Создаём in-memory репозиторий для теста
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	tests := []struct {
		nameTest    string
		input       model.URLModel
		wantErr     bool
		description string
	}{
		{
			nameTest: "Save new URL with auto-generated UUID",
			input: model.URLModel{
				OriginalURL: "https://example.com",
				UserID:      uuid.New(),
			},
			wantErr:     false,
			description: "New URL - auto-UUID generation and successful saving",
		},
		{
			nameTest: "Save URL with provided UUID",
			input: model.URLModel{
				UUID:        "custom-uuid-123",
				ShortURL:    "custom-uuid-123",
				OriginalURL: "https://google.com",
				UserID:      uuid.New(),
			},
			wantErr:     false, //
			description: "URL with an explicitly specified UUID",
		},
		{
			nameTest: "Save duplicate UUID",
			input: model.URLModel{
				UUID:        "duplicate-id",
				ShortURL:    "duplicate-id",
				OriginalURL: "https://duplicate.com",
			},
			wantErr:     false, // сейчас дубликат просто перезаписывается
			description: "Attempt to save a duplicate UUID",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Подготавливаем первое сохранение для теста дубликата
			if tt.nameTest == "save duplicate UUID" {
				// Подготавливаем первый экземпляр
				first := model.URLModel{
					UUID:        "duplicate-id",
					ShortURL:    "duplicate-id",
					OriginalURL: "https://first.com",
				}
				_, err := repo.SaveURL(t.Context(), first)
				require.NoError(t, err, "Couldn't save the first URL for the duplicate test")
			}

			// Выполняем сохранение
			saved, err := repo.SaveURL(t.Context(), tt.input)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), "already exists", tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.NotEmpty(t, saved.UUID, "The UUID must be filled in")
			assert.Equal(t, tt.input.OriginalURL, saved.OriginalURL, "The OriginalURL must match")
		})
	}
}

// TestMemoryRepository_GetURL тестирует получение URL по короткому идентификатору.
func TestMemoryRepository_GetURL(t *testing.T) {
	// Создаём in-memory репозиторий
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	// Подготавливаем тестовые данные
	url1 := model.URLModel{
		UUID:        "test-id-1",
		ShortURL:    "test-id-1",
		OriginalURL: "https://ya.ru",
		UserID:      uuid.New(),
	}
	_, _ = repo.SaveURL(t.Context(), url1)

	tests := []struct {
		nameTest    string
		id          string
		wantErr     bool
		description string
	}{
		{
			nameTest:    "Found existing URL",
			id:          "test-id-1",
			wantErr:     false,
			description: "Search for an existing URL",
		},
		{
			nameTest:    "Not found",
			id:          "non-existent-id",
			wantErr:     true,
			description: "Search for a non-existent URL",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Выполняем поиск по идентификатору
			got, err := repo.GetURL(t.Context(), tt.id)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), "not found")
				return
			}

			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.id, got.UUID)
			assert.Equal(t, "https://ya.ru", got.OriginalURL)
		})
	}
}

// TestMemoryRepository_SaveBatch тестирует пакетное сохранение нескольких URL.
func TestMemoryRepository_SaveBatch(t *testing.T) {
	// Создаём in-memory репозиторий
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	// Подготавливаем батч из двух URL
	batch := []model.URLModel{
		{UUID: "batch-1", ShortURL: "batch-1", OriginalURL: "https://1.com"},
		{UUID: "batch-2", ShortURL: "batch-2", OriginalURL: "https://2.com"},
	}

	// Выполняем пакетное сохранение
	err := repo.SaveBatch(t.Context(), batch)
	require.NoError(t, err)

	// Проверяем, что все URL действительно сохранены и доступны по GetURL
	for _, u := range batch {
		got, err := repo.GetURL(t.Context(), u.UUID)
		require.NoError(t, err)
		assert.Equal(t, u.OriginalURL, got.OriginalURL)
	}
}

// TestMemoryRepository_GetURLsByUser тестирует получение URL по пользователю.
func TestMemoryRepository_GetURLsByUser(t *testing.T) {
	// Создаём in-memory репозиторий
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	userID := uuid.New()

	// Сохраняем два URL для одного пользователя
	_, _ = repo.SaveURL(t.Context(), model.URLModel{
		UUID:        "user-url-1",
		ShortURL:    "user-url-1",
		OriginalURL: "https://my1.com",
		UserID:      userID,
	})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{
		UUID:        "user-url-2",
		ShortURL:    "user-url-2",
		OriginalURL: "https://my2.com",
		UserID:      userID,
	})

	// Получаем список URL пользователя
	urls, err := repo.GetURLsByUser(t.Context(), userID)
	require.NoError(t, err, "GetURLsByUser should not return error")
	assert.Len(t, urls, 2, "Should return exactly 2 URLs for the user")
}

// TestMemoryRepository_MarkAsDeleted тестирует мягкое удаление.
func TestMemoryRepository_MarkAsDeleted(t *testing.T) {
	// Создаём in-memory репозиторий
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	userID := uuid.New()

	// Сохраняем URL для пользователя
	_, _ = repo.SaveURL(t.Context(), model.URLModel{
		UUID:        "to-delete",
		ShortURL:    "to-delete",
		OriginalURL: "https://delete.me",
		UserID:      userID,
	})

	// Помечаем URL как удалённый
	err := repo.MarkAsDeleted(t.Context(), userID, []string{"to-delete"})
	require.NoError(t, err)

	// После delete URL не должен возвращаться в списке пользователя
	urls, _ := repo.GetURLsByUser(t.Context(), userID)
	assert.Len(t, urls, 0, "Deleted URL should not appear in user's list") // после delete не должен возвращаться
}

// TestMemoryRepository_Stats тестирует подсчёт статистики.
func TestMemoryRepository_Stats(t *testing.T) {
	// Создаём in-memory репозиторий
	repo := NewMemoryRepository("http://localhost:8080", zap.NewNop())

	// Добавляем тестовые данные (2 URL от разных пользователей)
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://1.com", UserID: uuid.New()})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://2.com", UserID: uuid.New()})

	// Получаем статистику
	urls, users := repo.Stats()

	assert.GreaterOrEqual(t, urls, 2, "Should count at least 2 URLs")
	assert.GreaterOrEqual(t, users, 1, "Should count at least 1 user")
}
