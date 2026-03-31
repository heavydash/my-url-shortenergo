// Пакет grpc предоставляет gRPC-сервер, работающий параллельно с HTTP-сервером.
//
// Пакет реализует полноценный gRPC-интерфейс для сервиса сокращения URL,
// используя общий бизнес-слой (internal/service). Это позволяет другим сервисам
// взаимодействовать с shortener через протокол gRPC, а не только через HTTP.
//
// # Основные компоненты пакета
//
//   - Server — управляет жизненным циклом gRPC-сервера (запуск и graceful stop).
//   - AuthInterceptor — unary-интерцептор, проверяющий авторизацию через metadata.
//   - shortenerServer — реализация protobuf-сервиса ShortenerServiceServer.
// # Поддерживаемые методы
//
//   - ShortenURL      — создание короткой ссылки.
//   - ExpandURL       — получение оригинального URL по короткому идентификатору.
//   - ListUserURLs    — получение списка ссылок текущего пользователя.

package grpc

import (
	"context"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/service"
	"github.com/heavydash/my-url-shortenergo/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
)

// Server представляет gRPC-сервер сервиса сокращения URL.
//
// Server инкапсулирует *grpc.Server, конфигурацию и логгер. Он отвечает за
// запуск сервера в отдельной горутине и корректную остановку через GracefulStop.
type Server struct {
	cfg     *config.Config
	logger  *zap.Logger
	grpcSrv *grpc.Server
}

// NewServer создаёт и настраивает новый gRPC-сервер.
//
// Функция принимает бизнес-сервис URLService, конфигурацию и логгер.
// Внутри она:
//   - создаёт реализацию protobuf-сервиса (shortenerServer),
//   - настраивает unary-интерцептор авторизации,
//   - регистрирует сервис и reflection.
//
// Возвращает готовый Server, готовый к вызову Start().
func NewServer(svc service.URLService, cfg *config.Config, logger *zap.Logger) *Server {
	// Создаём реализацию сервиса
	serviceImpl := NewShortenerService(svc, cfg)

	// Создаём gRPC сервер
	grpcSrv := grpc.NewServer(
		grpc.UnaryInterceptor(AuthInterceptor(logger)))

	// Регистрируем наш сервис
	shortener.RegisterShortenerServiceServer(grpcSrv, serviceImpl)

	// Включаем Reflection
	reflection.Register(grpcSrv)

	return &Server{
		grpcSrv: grpcSrv,
		cfg:     cfg,
		logger:  logger,
	}
}

// Start запускает gRPC-сервер на указанном адресе в отдельной горутине.
//
// Метод слушает TCP-порт (временно жёстко задан :9090, в будущем будет браться из конфигурации)
// и запускает Serve в фоне. При возникновении ошибки сервер логирует её, но не падает.
//
// Start возвращает nil сразу после запуска горутины. Ошибки запуска listener’а
// возвращаются синхронно.
func (s *Server) Start() error {
	addr := s.cfg.GRPCAddr
	if addr == "" {
		addr = ":9090"
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.logger.Info("gRPC server starting", zap.String("address", addr))

	go func() {
		if err := s.grpcSrv.Serve(lis); err != nil {
			s.logger.Error("gRPC server failed", zap.Error(err))
		}
	}()

	s.logger.Info("gRPC server started successfully", zap.String("address", addr))
	return nil
}

// Stop gracefully останавливает gRPC-сервер.
//
// Метод вызывает GracefulStop(), который дожидается завершения всех активных
// RPC-вызовов. Подходит для использования при graceful shutdown всего приложения.
//
// ctx в текущей реализации не используется (GracefulStop не поддерживает контекст),
// но оставлен для совместимости с общим интерфейсом shutdown.
func (s *Server) Stop(ctx context.Context) error {
	if s.grpcSrv != nil {
		s.logger.Info("stopping gRPC server...")
		s.grpcSrv.GracefulStop()
	}
	return nil
}
