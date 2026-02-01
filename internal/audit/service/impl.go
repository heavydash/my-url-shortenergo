package service

import (
	"context"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"go.uber.org/zap"
	"sync"
	"time"
)

// Основная асинхронная реализация Аудита
type AuditService struct {
	cfg      *config.Config
	logger   *zap.Logger
	eventCh  chan *audit.Event // буфферизированный канал событий
	senders  []sender.Sender   // все активные отправители
	once     sync.Once         // гарантия однократного запуска воркера
	shutdown chan struct{}     // сигнал graceful shutdown
	closed   chan struct{}     // сигнал завершения воркера
}

// Эксземпляр сервиса аудита
func NewAuditService(cfg *config.Config, logger *zap.Logger) *AuditService {
	s := &AuditService{
		cfg:      cfg,
		logger:   logger,
		eventCh:  make(chan *audit.Event, 4096),
		senders:  make([]sender.Sender, 0, 2),
		shutdown: make(chan struct{}),
		closed:   make(chan struct{}),
	}

	if cfg.AuditFilePath != "" {
		s.senders = append(s.senders, sender.NewFileSender(cfg.AuditFilePath, s.logger))
	}

	if cfg.AuditRemoteURL != "" {
		s.senders = append(s.senders, sender.NewHTTPSender(cfg.AuditRemoteURL, s.logger))
	}

	if len(s.senders) == 0 {
		s.logger.Warn("audit configured without any senders")
	}
	// Запускаем воркер один раз, защищая от задваивания и race
	s.once.Do(func() {
		go s.worker()
		s.logger.Info("audit service started",
			zap.Int("buffer_capacity", cap(s.eventCh)),
			zap.Int("senders_count", len(s.senders)),
		)
	})
	return s
}

// Добавление события в очередь
func (s *AuditService) SendAsync(event *audit.Event) {
	if event == nil {
		s.logger.Debug("nil event received")
		return
	}

	select {
	case s.eventCh <- event:
	case <-s.shutdown:
		s.logger.Info("audit service shutdown")
	case <-s.closed:
		s.logger.Info("audit service closed")
	default:
		s.logger.Warn("audit service is full",
			zap.String("action", event.Action),
			zap.String("url", event.URL),
			zap.String("user_id", event.UserID),
		)
	}
}

// Gracefull Shutdown
func (s *AuditService) Shutdown(ctx context.Context) error {
	s.logger.Info("audit service shutdown")

	// Прекращаем принимать новые события
	close(s.shutdown)

	// Воркер дорабатывает остаток очереди
	select {
	case <-s.closed:
		s.logger.Info("audit service gracefully shutdown")
		return nil
	case <-ctx.Done():
		s.logger.Warn("audit shutdown timed out", zap.Duration("elapsed",
			time.Since(time.Now())))
		return ctx.Err()
	}
}

// worker goroutine
func (s *AuditService) worker() {
	defer close(s.closed)
	defer s.logger.Info("audit worker shutdown")

	s.logger.Info("audit worker started")

	for {
		select {
		case event, ok := <-s.eventCh:
			if !ok {
				s.logger.Info("event channel closed")
				return
			}
			// рассылаем сообщение всем sender
			for _, snd := range s.senders {
				if err := snd.Send(event); err != nil {
					s.logger.Error("failed to send event",
						zap.String("sender", snd.Name()),
						zap.String("action", event.Action),
						zap.String("url", event.URL),
						zap.String("user_id", event.UserID),
						zap.Error(err),
					)
				}
			}
		case <-s.shutdown:
			s.logger.Info("audit worker shutdown")
			for event := range s.eventCh {
				for _, snd := range s.senders {
					if err := snd.Send(event); err != nil {
						s.logger.Error("audit send failed during shutdown",
							zap.String("sender", snd.Name()),
							zap.Error(err),
						)
					}
				}
			}
			s.logger.Info("queue drained, worker shutdown completed")

			// Закрытие отправителей (файлов и соединений)
			for _, snd := range s.senders {
				if closer, ok := snd.(interface{ Close() error }); ok {
					if err := closer.Close(); err != nil {
						s.logger.Error("sender closed failed during shutdown",
							zap.String("sender", snd.Name()),
							zap.Error(err),
						)
					}
				}
			}

			return
		}
	}
}
