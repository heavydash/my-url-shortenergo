// Package sender предоставляет интерфейсы и реализации отправителей аудит-событий.
// Пакет реализует паттерн "стратегия" для отправки событий в различные системы:
// файлы, HTTP эндпоинты, message queues, базы данных и т.д.
package sender

import "github.com/heavydash/my-url-shortenergo/internal/audit"

// Sender — базовый контракт отправителя аудит-событий. определяет интерфейс для отправки аудит-событий в различные системы назначения.
//
// Реализует стратегию доставки событий в разные системы.
// Все реализации должны быть потокобезопасными.
//
// Основные реализации:
//   - FileSender: запись в локальный файл
//   - HTTPSender: отправка по HTTP/HTTPS
//   - ConsoleSender: вывод в stdout/stderr (отладка)
//
// Пример использования через интерфейс:
//
//			fileSender := sender.NewFileSender(cfg.AuditFilePath, logger)
//		    httpSender := sender.NewHTTPSender(cfg.AuditRemoteURL, cfg.HTTPClientTimeout, logger)
//
//			var senders []sender.Sender
//			senders = append(senders, fileSender)
//			senders = append(senders, httpSender)
//
//			for _, s := range senders {
//			   if err := s.Send(event); err != nil {
//	            log.Printf("Failed to send via %s: %v", s.Name(), err)
//	        }
//	    }
type Sender interface {
	// Name возвращает уникальное имя отправителя.
	//
	// Используется для:
	//   - Идентификации отправителя в логах
	//   - Мониторинга и метрик (например, prometheus labels)
	//   - Конфигурации и динамического управления
	//
	// Примеры имен:
	//   - "file:/var/log/audit.log"
	//   - "http:https://audit.internal/api/events"
	//
	// Возвращает:
	//   - string: уникальное имя отправителя
	Name() string

	// Send отправляет аудит-событие в целевую систему.
	//
	// Параметры:
	//   - event: указатель на аудит-событие для отправки.
	//
	//
	// Возвращает:
	//   - error: ошибка отправки или nil при успехе.
	//
	Send(event *audit.Event) error
}

// CloserSender — расширенный контракт для отправителей, которые требуют закрытия ресурсов.
// Встраивает Sender и добавляет Close().
//
// Примеры:
//   - FileSender: закрывает файл
//   - HTTPSender: обычно noop (return nil)
//   - DatabaseSender: закрывает соединение
//
// Используется в graceful shutdown для корректного освобождения ресурсов.
type CloserSender interface {
	Sender
	Close() error
}
