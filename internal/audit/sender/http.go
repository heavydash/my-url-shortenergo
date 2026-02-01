package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type HTTPSender struct {
	url    string
	client *http.Client
	logger *zap.Logger
}

func NewHTTPSender(url string, logger *zap.Logger) *HTTPSender {
	return &HTTPSender{
		url: url,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger.Named("http-sender"),
	}
}

func (s *HTTPSender) Name() string {
	return fmt.Sprintf("http:%s", s.url)
}

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
