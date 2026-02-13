// Package sender предоставляет реализации отправителей аудит-событий.
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"go.uber.org/zap"
)

// HTTPSender реализует отправку аудит-событий по HTTP протоколу.
//
// Особенности:
//   - Отправка событий в формате JSON через HTTP POST
//   - Таймаут запроса: 5 секунд (настраивается)
//   - Проверка HTTP статус-кодов ответа
//   - Автоматическая установка Content-Type: application/json
//   - Контекст с таймаутом для предотвращения блокировок
//
// Используется для:
//   - Интеграции с внешними системами мониторинга
//   - Централизованного сбора аудит-логов
//   - Отправки событий в SIEM системы (Splunk, ELK, Graylog)
//   - Вебхуков и нотификаций
//
// Пример запроса:
//
//	POST /api/audit/events HTTP/1.1
//	Content-Type: application/json
//
//	{
//	  "timestamp": "2024-01-15T10:30:00Z",
//	  "user_id": "user-123",
//	  "action": "shorten",
//	  "details": "{\"url\":\"https://example.com\"}"
//	}
type HTTPSender struct {
	url     string
	client  *http.Client
	logger  *zap.Logger
	timeout time.Duration
}

func (s *HTTPSender) Close() error {
	return nil
}

// NewHTTPSender создает новый экземпляр HTTPSender.
//
// Инициализирует HTTP клиент с разумными таймаутами для production использования.
// Рекомендуется использовать HTTPS протокол для защиты передаваемых данных.
//
// Параметры:
//
//   - url: полный URL целевого эндпоинта (например, "https://audit.example.com/api/events")
//   - timeout: таймаут HTTP запроса из cfg.HTTPClientTimeout
//   - logger: логгер для записи внутренних событий отправителя
//
// Возвращает:
//   - *HTTPSender: готовый к использованию отправитель
//
// Пример использования:
//
//	logger, _ := zap.NewProduction()
//	sender := sender.NewHTTPSender("https://audit.internal/api/events", logger)
//
// Пример использования:
//
//	logger, _ := zap.NewProduction()
//	sender := sender.NewHTTPSender("https://audit.internal/api/events", cfg.HTTPClientTimeout, logger)
func NewHTTPSender(url string, timeout time.Duration, logger *zap.Logger) *HTTPSender {
	return &HTTPSender{
		url:     url,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger.Named("http-sender"),
	}
}

// Name возвращает уникальное имя отправителя.
// Используется для идентификации отправителя в логах и метриках.
//
// Возвращает:
//   - string: имя в формате "http:{URL}"
//
// Пример:
//
//	sender.Name() // "http:https://audit.example.com/api/events"
func (s *HTTPSender) Name() string {
	return fmt.Sprintf("http:%s", s.url)
}

// Send отправляет аудит-событие на удаленный сервер по HTTP.
//
// Метод выполняет:
//  1. Сериализацию события в JSON
//  2. Создание HTTP POST запроса с контекстом
//  3. Установку заголовка Content-Type: application/json
//  4. Отправку запроса с таймаутом
//  5. Проверку HTTP статус-кода ответа
//  6. Логирование успешной отправки
//
// Параметры:
//   - event: аудит-событие для отправки
//
// Возвращает:
//   - error: ошибка если не удалось отправить событие
func (s *HTTPSender) Send(event *audit.Event) error {
	// Сериализация события в JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	// Создание POST запроса
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		s.url,
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for audit: %w", err)
	}

	// Заголовок для JSON
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	s.logger.Debug("audit event sent via http",
		zap.String("url", s.url),
		zap.String("action", event.Action),
		zap.String("user_id", event.UserID),
	)

	return nil
}
