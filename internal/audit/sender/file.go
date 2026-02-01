package sender

import (
	"encoding/json"
	"github.com/heavydash/my-url-shortenergo/internal/audit"
	"go.uber.org/zap"
	"os"
	"sync"
)

type FileSender struct {
	path   string
	file   *os.File
	mu     sync.Mutex
	logger *zap.Logger
}

func NewFileSender(path string, logger *zap.Logger) *FileSender {
	return &FileSender{
		path:   path,
		logger: logger.Named("file_sender"),
	}
}

func (s *FileSender) Name() string {
	return "file" + s.path
}

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
