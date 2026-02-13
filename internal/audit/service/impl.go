// Package service предоставляет основную реализацию сервиса аудита.
// Сервис реализует асинхронную обработку аудит-событий с буферизацией,
// поддержкой multiple sender'ов и graceful shutdown.
package service

import (
	"context"
	"github.com/heavydash/my-url-shortenergo/internal/audit/sender"
	"sync"
	"time"

	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"go.uber.org/zap"
)

// AuditService реализует асинхронный сервис аудита с буферизацией событий.
//
// Архитектура:
//   - Producer-Consumer паттерн с буферизированным каналом
//   - Асинхронная обработка (не блокирует основной поток)
//   - Поддержка multiple sender'ов (файл, HTTP, и т.д.)
//   - Graceful shutdown с drain'ом очереди
//   - Защита от race condition через sync.Once
//
// Основные компоненты:
//   - eventCh: буферизированный канал для событий (capacity: 4096)
//   - worker: горутина-обработчик событий
//   - senders: список отправителей для доставки событий
//   - shutdown/closed: каналы для graceful shutdown
//
// Пример потока данных:
//  1. Handler вызывает SendAsync(event)
//  2. Событие помещается в eventCh
//  3. Worker читает события из канала
//  4. Worker отправляет событие во все зарегистрированные sender'ы
//  5. Sender'ы доставляют события в целевые системы
type AuditService struct {
	cfg      *config.Config
	logger   *zap.Logger
	eventCh  chan *audit.Event // буфферизированный канал событий
	senders  []sender.Sender   // все активные отправители
	once     sync.Once         // гарантия однократного запуска воркера
	shutdown chan struct{}     // сигнал graceful shutdown
	closed   chan struct{}     // сигнал завершения воркера
}

// NewAuditService создает и запускает новый экземпляр AuditService.
//
// Параметры:
//   - cfg: конфигурация приложения (используется AuditBufferSize)
//   - logger: структурированный логгер
//   - senders: вариативный список отправителей (FileSender, HTTPSender и т.д.)
//
// Возвращает:
//   - *AuditService: готовый к использованию сервис аудита
//
// Пример использования:
//
//	cfg := config.Load()
//	logger, _ := zap.NewProduction()
//
//	var senders []sender.Sender
//	if cfg.AuditFilePath != "" {
//	    senders = append(senders, sender.NewFileSender(cfg.AuditFilePath, logger))
//	}
//	if cfg.AuditRemoteURL != "" {
//	    senders = append(senders, sender.NewHTTPSender(cfg.AuditRemoteURL, cfg.HTTPClientTimeout, logger))
//	}
//
//	auditSvc := service.NewAuditService(cfg, logger, senders...)
//	defer auditSvc.Shutdown(context.Background())
func NewAuditService(cfg *config.Config,
	logger *zap.Logger,
	senders ...sender.Sender,
) *AuditService {
	s := &AuditService{
		cfg:      cfg,
		logger:   logger,
		eventCh:  make(chan *audit.Event, cfg.AuditBufferSize),
		senders:  make([]sender.Sender, 0, len(senders)),
		shutdown: make(chan struct{}),
		closed:   make(chan struct{}),
	}

	for _, snd := range senders {
		if snd != nil {
			s.senders = append(s.senders, snd)
		}
	}

	if len(s.senders) == 0 {
		s.logger.Warn("audit configured without any senders")
	}
	// Запускаем воркер один раз, защищая от задваивания и race
	s.once.Do(func() {
		go s.worker()
		s.logger.Info("audit service started",
			zap.Int("buffer_capacity", cap(s.eventCh)),
			zap.Int("senders_count", len(senders)),
		)
	})
	return s
}

// SendAsync асинхронно отправляет аудит-событие на обработку.
//
// Метод помещает событие в буферизированную очередь и немедленно возвращает управление.
//
// Параметры:
//   - event: аудит-событие для записи
//
// Особенности:
//   - Не блокирующий вызов
//   - Thread-safe
//   - При переполнении буфера выводится warning и событие отбрасывается
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

// Shutdown выполняет graceful shutdown сервиса аудита.
//
// Процесс shutdown:
//  1. Прекращает прием новых событий
//  2. Дожидается обработки всех событий в буфере
//  3. Закрывает все sender'ы (файлы, соединения)
//  4. Гарантирует освобождение ресурсов
//
// Параметры:
//   - ctx: контекст с таймаутом (рекомендуется cfg.AuditShutdownTimeout)
//
// Возвращает:
//   - error: ошибка если shutdown превысил таймаут контекста
//
// Пример использования:
//
//	ctx, cancel := context.WithTimeout(context.Background(), cfg.AuditShutdownTimeout)
//	defer cancel()
//	if err := auditSvc.Shutdown(ctx); err != nil {
//	    logger.Error("audit shutdown failed", zap.Error(err))
//	}
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

// worker - фоновый обработчик аудит-событий.
//
// Горутина выполняет:
//  1. Чтение событий из буферизированного канала
//  2. Отправку каждого события во все зарегистрированные sender'ы
//  3. Обработку graceful shutdown с drain'ом очереди
//  4. Корректное закрытие sender'ов
//
// Внутренняя логика:
//   - Бесконечный цикл с select по каналам
//   - При shutdown: обработка оставшихся событий (drain)
//   - Закрытие всех sender'ов, реализующих интерфейс Closer
//   - Защита от паники (рекомендуется добавить recover)
//
// Примечания для разработки:
//   - Worker гарантированно запускается один раз через sync.Once
//   - При панике worker'а сервис перестает обрабатывать события
//   - Рассмотрите добавление метрик: обработки/сек, ошибок отправки
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
				err := snd.Send(event)
				if err != nil {
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
					snd.Send(event)

				}
			}
			s.logger.Info("queue drained, worker shutdown completed")

			// Закрытие отправителей (файлов и соединений)
			for _, snd := range s.senders {
				if closer, ok := snd.(sender.CloserSender); ok {
					err := closer.Close()
					if err != nil {
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
