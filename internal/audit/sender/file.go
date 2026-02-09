// Package sender предоставляет реализации отправителей аудит-событий.
// Поддерживаются различные способы доставки событий: файлы, сетевые сокеты,
// брокеры сообщений и т.д.
package sender

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"go.uber.org/zap"
)

// FileSender реализует отправку аудит-событий в файл.
//
// Особенности:
//   - Запись событий в формате JSON, каждое событие на новой строке
//   - Автоматическое создание файла при первом использовании
//   - Гарантированная запись на диск через file.Sync()
//   - Потокобезопасность через мьютекс
//   - Ленивая инициализация файлового дескриптора
//
// Используется для:
//   - Отладки и разработки
//   - Локального сбора логов
//   - Систем без доступа к сетевым хранилищам
//
// Пример содержимого файла:
//
//	{"timestamp":"2024-01-15T10:30:00Z","user_id":"123","action":"shorten","details":"...\n"}
//	{"timestamp":"2024-01-15T10:30:05Z","user_id":"123","action":"follow","details":"...\n"}
type FileSender struct {
	path   string
	file   *os.File
	mu     sync.Mutex
	logger *zap.Logger
}

// NewFileSender создает новый экземпляр FileSender.
//
// Файл не открывается сразу - открытие происходит при первой отправке события.
// Это позволяет создавать отправители заранее, не беспокоясь о доступности файловой системы.
//
// Параметры:
//   - path: абсолютный или относительный путь к файлу аудита
//   - logger: логгер для записи внутренних событий отправителя
//
// Возвращает:
//   - *FileSender: готовый к использованию отправитель
//
// Пример использования:
//
//	logger, _ := zap.NewDevelopment()
//	sender := sender.NewFileSender("/var/log/url-shortener/audit.log", logger)
//	defer sender.Close()
//
// Примечания:
//   - Файл создается с правами 0644 (rw-r--r--)
//   - Если файл существует, события дописываются в конец (режим append)
//   - Рекомендуется использовать абсолютные пути для production
func NewFileSender(path string, logger *zap.Logger) *FileSender {
	return &FileSender{
		path:   path,
		logger: logger.Named("file_sender"),
	}
}

// Name возвращает уникальное имя отправителя.
// Используется для идентификации отправителя в логах и метриках.
//
// Возвращает:
//   - string: имя в формате "file:{путь_к_файлу}"
//
// Пример:
//
//	sender.Name() // "file:/var/log/audit.log"
func (s *FileSender) Name() string {
	return "file" + s.path
}

// Send отправляет аудит-событие в файл.
//
// Метод потокобезопасен и реализует ленивую инициализацию файла.
// Каждое событие записывается в формате JSON с последующим переводом строки.
//
// Параметры:
//   - event: аудит-событие для записи
//
// Возвращает:
//   - error: ошибка если не удалось записать событие
//
// Пример события в файле:
//
//	{"timestamp":"2024-01-15T10:30:00Z","user_id":"user-123","action":"shorten","details":"{\"url\":\"https://example.com\"}"}\n
//
// Коды ошибок:
//   - ошибки открытия/создания файла
//   - ошибки сериализации JSON
//   - ошибки записи на диск
func (s *FileSender) Send(event *audit.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			s.logger.Error("failed to open file",
				zap.String("path", s.path),
				zap.Error(err))
			return err
		}
		s.file = f
		s.logger.Info("audit file opened", zap.String("path", s.path))
	}

	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("failed to marshal audit event")
		return err
	}
	_, err = s.file.Write(data)
	if err != nil {
		s.logger.Error("failed to write audit event")
		return err
	}
	return s.file.Sync() // гарантированная доставка файлов
}

// Close закрывает файловый дескриптор.
//
// Должен вызываться при завершении работы приложения для корректного
// освобождения ресурсов. Реализует идемпотентность - повторные вызовы
// не приводят к ошибкам.
//
// Возвращает:
//   - error: ошибка закрытия файла, если произошла
//
// Примечания:
//   - Все незаписанные буферы сбрасываются на диск
//   - Файловый дескриптор освобождается
//   - После Close() Send() снова откроет файл при необходимости
func (s *FileSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		if err != nil {
			s.logger.Info("failed to close file",
				zap.String("path", s.path),
				zap.Error(err))
			return err
		}
		s.logger.Info("closed file", zap.String("path", s.path))
	}
	return nil
}
