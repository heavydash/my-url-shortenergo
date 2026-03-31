package grpc

import (
	"context"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor возвращает unary-серверный интерцептор для проверки авторизации в gRPC.
//
// Интерцептор работает аналогично HTTP-middleware.Auth, но использует gRPC metadata
// вместо HTTP-заголовков. Он:
//   - извлекает metadata из входящего контекста,
//   - ищет заголовок "authorization",
//   - парсит его через middleware.ParseAuthHeader,
//   - при успешной проверке кладёт userID в контекст под ключом middleware.UserIDKey,
//   - возвращает статус Unauthenticated при любой ошибке.
//
// Используется при создании gRPC-сервера в NewServer().
func AuthInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {

		// Извлекаем metadata из контекста
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		// Ищем заголовок authorization
		authValues := md.Get("authorization")
		if len(authValues) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		logger.Info("gRPC auth header received", zap.String("authorization", authValues[0]))

		// Переиспользуем логику из HTTP-middleware
		userID, err := middleware.ParseAuthHeader(authValues[0])
		if err != nil {
			logger.Warn("invalid authorization header in gRPC", zap.Error(err))
			return nil, status.Error(codes.Unauthenticated, "invalid authorization")
		}

		// Кладём userID в контекст (точно так же, как в HTTP)
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)

		// Продолжаем обработку
		return handler(ctx, req)
	}
}
