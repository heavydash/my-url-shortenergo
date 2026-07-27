// Package repository содержит тесты для PostgreSQL-реализации хранилища URL.
//
// Тесты используют реальную базу данных (требуется DATABASE_DSN) и helper newTestPostgresRepo(),
// который создаёт пул, очищает таблицу и правильно закрывает ресурсы после теста.
//
// Все тесты проверяют поведение, специфичное для PostgreSQL: уникальность ограничений,
// обработку конфликтов, транзакции и soft delete.
package repository

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestPostgresRepo(t *testing.T) *PostgresRepository {
	t.Helper()

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN environment variable not set")
	}

	// Создаём контекст с таймаутом для инициализации
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Простое прямое создание пула через pgxpool
	poolConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err, "failed to parse DSN")

	// Создаём пул соединений
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig failed: %v", err)
	}

	logger := zap.NewNop()

	repo := NewPostgresRepository(pool, logger, "http://localhost:8080")

	// Очищаем таблицу перед тестами
	_, err = pool.Exec(ctx, "TRUNCATE TABLE urls RESTART IDENTITY CASCADE")
	if err != nil {
		t.Logf("Warning: TRUNCATE failed: %v", err)
	}

	// Закрываем пул после завершения теста
	t.Cleanup(func() {
		pool.Close()
	})

	t.Log("Test Postgres repo ready")
	return repo
}

// TestPostgresRepository_SaveURL проверяет сохранение одиночного URL в PostgreSQL.
//
// Особенности PostgresRepository:
//   - При дубликате short_url или uuid возвращается ошибка уникальности
//   - Автоматически генерирует ID и short_url при необходимости
//   - Использует ON CONFLICT DO NOTHING + последующий SELECT
func TestPostgresRepository_SaveURL(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	tests := []struct {
		nameTest    string
		input       model.URLModel
		wantErr     bool
		description string
	}{
		{
			nameTest: "Save URL with provided UUID and ShortURL",
			input: model.URLModel{
				UUID:        "pg-uuid-123",
				ShortURL:    "pg-uuid-123",
				OriginalURL: "https://google.com",
				UserID:      uuid.New(),
			},
			wantErr:     false,
			description: "A URL with an explicitly specified UUID and ShortURL",
		},
		{
			nameTest: "Save duplicate UUID",
			input: model.URLModel{
				UUID:        "pg-duplicate",
				ShortURL:    "pg-duplicate",
				OriginalURL: "https://duplicate.com",
			},
			wantErr:     true,
			description: "Duplicate short_url, Postgres should return a uniqueness error.",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {

		t.Run(tt.nameTest, func(t *testing.T) {

			// Подготавливаем первое сохранение для теста дубликата
			if tt.nameTest == "Save duplicate UUID" {
				first := model.URLModel{
					UUID:        "pg-duplicate",
					ShortURL:    "pg-duplicate",
					OriginalURL: "https://first.com",
				}
				_, err := repo.SaveURL(t.Context(), first)
				require.NoError(t, err, "Couldn't save the first instance")
			}

			// Выполняем сохранение
			saved, err := repo.SaveURL(t.Context(), tt.input)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				// Postgres возвращает свою ошибку уникальности
				assert.Contains(t, err.Error(), "duplicate key", tt.description)
				assert.Contains(t, err.Error(), "urls_short_url_key", tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.NotEmpty(t, saved.UUID, "The UUID must be generated")
			assert.NotEmpty(t, saved.ShortURL, "ShortURL must be filled in")
			assert.Equal(t, tt.input.OriginalURL, saved.OriginalURL)
		})
	}
}

// TestPostgresRepository_GetURL тестирует получение URL по короткому идентификатору из PostgreSQL.
func TestPostgresRepository_GetURL(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	// Подготавливаем тестовые данные
	saved, err := repo.SaveURL(t.Context(), model.URLModel{
		OriginalURL: "https://ya.ru",
		UserID:      uuid.New(),
	})
	require.NoError(t, err, "The test URL could not be saved")

	tests := []struct {
		nameTest    string
		shortID     string
		wantErr     bool
		description string
	}{
		{
			nameTest:    "Found existing URL",
			shortID:     saved.ShortURL,
			wantErr:     false,
			description: "The existing URL must be found successfully",
		},
		{
			nameTest:    "Not found",
			shortID:     "non-existent-id",
			wantErr:     true,
			description: "A non-existent short_url should return an error.",
		},
	}

	// Перебираем все сценарии
	for _, tt := range tests {
		t.Run(tt.nameTest, func(t *testing.T) {

			// Выполняем поиск по идентификатору
			got, err := repo.GetURL(t.Context(), tt.shortID)

			if tt.wantErr {
				require.Error(t, err, tt.description)
				assert.Contains(t, strings.ToLower(err.Error()), "not found", tt.description)
				return
			}

			require.NoError(t, err, tt.description)
			assert.Equal(t, saved.ID, got.ID)
			assert.Equal(t, saved.ShortURL, got.ShortURL)
			assert.Equal(t, saved.OriginalURL, got.OriginalURL)
			assert.False(t, got.IsDeleted)
		})
	}
}

// TestPostgresRepository_SaveBatch тестирует пакетное сохранение URL в одной транзакции
func TestPostgresRepository_SaveBatch(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	// Подготавливаем батч из трёх URL
	batch := []model.URLModel{
		{OriginalURL: "https://github.com/test-batch-1-" + uuid.New().String(), UserID: uuid.New()},
		{OriginalURL: "https://gitlab.com/test-batch-2-" + uuid.New().String(), UserID: uuid.New()},
		{OriginalURL: "https://bitbucket.org/test-batch-3-" + uuid.New().String(), UserID: uuid.New()},
	}

	// Выполняем пакетное сохранение
	err := repo.SaveBatch(t.Context(), batch)
	require.NoError(t, err, "SaveBatch should pass without error")

	// Проверяем, что все URL успешно сохранены
	for i := range batch {
		shortURL := batch[i].ShortURL
		t.Logf("Checking item %d with ShortURL: %s", i, shortURL)

		got, err := repo.GetURL(t.Context(), shortURL)
		if err != nil {
			t.Logf("GetURL failed for item %d: %v", i, err)
			t.Fatalf("Couldn't find the saved URL from the batch for item %d", i)
		}

		assert.Equal(t, batch[i].OriginalURL, got.OriginalURL, "The OriginalURL must match")
		assert.Equal(t, shortURL, got.ShortURL, "ShortURL must match")
		assert.NotEmpty(t, got.ID, "The ID must be generated")
	}
}

// TestPostgresRepository_GetURLsByUser тестирует получение всех URL конкретного пользователя
func TestPostgresRepository_GetURLsByUser(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	userID := uuid.New()

	// Сохраняем URL двух пользователей
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://ya.ru", UserID: userID})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://google.com", UserID: userID})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://vk.com", UserID: uuid.New()}) // другой пользователь

	// Получаем список URL пользователя
	urls, err := repo.GetURLsByUser(t.Context(), userID)
	require.NoError(t, err)

	assert.Len(t, urls, 2, "Exactly 2 URLs of this user should be returned")
}

// TestPostgresRepository_MarkAsDeleted тестирует мягкое удаление URL
func TestPostgresRepository_MarkAsDeleted(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	userID := uuid.New()
	ids := []string{"del1", "del2"}

	// Сохраняем два URL
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

// TestPostgresRepository_Stats тестирует подсчёт статистики
func TestPostgresRepository_Stats(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	// Добавляем два URL от разных пользователей
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://ya.ru", UserID: uuid.New()})
	_, _ = repo.SaveURL(t.Context(), model.URLModel{OriginalURL: "https://google.com", UserID: uuid.New()})

	urlsCount, usersCount := repo.Stats()
	assert.Equal(t, 2, urlsCount, "There must be 2 URLs")
	assert.Equal(t, 2, usersCount, "There must be 2 unique users")
}

// TestPostgresRepository_Ping тестирует проверку доступности базы данных
func TestPostgresRepository_Ping(t *testing.T) {
	// Создаём тестовый Postgres-репозиторий
	repo := newTestPostgresRepo(t)

	// Выполняем ping
	err := repo.Ping(t.Context())
	require.NoError(t, err, "Ping should take place when the database is live")
}
