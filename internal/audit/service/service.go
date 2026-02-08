package service

import (
	"context"

	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"go.uber.org/zap"
)

// Контракт сервиса Аудита
type Service interface {
	SendAsync(event *audit.Event)
	Shutdown(ctx context.Context) error
}

func New(cfg *config.Config, logger *zap.Logger) Service {
	if cfg.AuditFilePath == "" && cfg.AuditRemoteURL == "" {
		return Noop{}
	}

	return NewAuditService(cfg, logger)
}

type Noop struct{}

func (Noop) SendAsync(_ *audit.Event)         {}
func (Noop) Shutdown(_ context.Context) error { return nil }
