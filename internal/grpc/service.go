package grpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/service"

	"github.com/heavydash/my-url-shortenergo/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/url"
)

// shortenerServer реализует protobuf-интерфейс ShortenerServiceServer (Opaque API).
//
// Это фасад между gRPC-транспортом и общим бизнес-слоем (URLService).
// Все методы:
//   - используют только Get*/Set* методы сгенерированных сообщений (Opaque API)
//   - извлекают userID через middleware.GetUserID (единый способ для HTTP и gRPC)
//   - возвращают полные URL через url.JoinPath (корректная конкатенация)
//
// Контекст может содержать userID, установленный AuthInterceptor.
type shortenerServer struct {
	shortener.UnimplementedShortenerServiceServer

	cfg    *config.Config
	svc    service.URLService
	logger *zap.Logger
}

// NewShortenerService создаёт новую реализацию gRPC-сервиса.
//
// Параметры:
//   - svc    — общий бизнес-сервис (URLService)
//   - cfg    — конфигурация (BaseURL и т.д.)
//   - logger — логгер для ошибок и отладки
//
// Возвращает объект, готовый к регистрации в grpc.NewServer().
func NewShortenerService(svc service.URLService, cfg *config.Config, logger *zap.Logger) shortener.ShortenerServiceServer {
	return &shortenerServer{
		svc:    svc,
		cfg:    cfg,
		logger: logger,
	}
}

// ShortenURL реализует rpc ShortenURL (аналог POST /api/shorten).
//
// Логика:
//  1. Проверяем наличие url
//  2. Извлекаем userID из контекста (может быть Nil)
//  3. Сохраняем через общий сервис
//  4. Формируем полный короткий URL и возвращаем через SetResult
//
// Возможные ошибки:
//   - InvalidArgument — url пустой
//   - Internal       — ошибка в URLService
func (s *shortenerServer) ShortenURL(ctx context.Context, req *shortener.URLShortenRequest) (*shortener.URLShortenResponse, error) {
	if req.GetUrl() == "" {
		return nil, status.Error(codes.InvalidArgument, "url is required")
	}

	// Извлекаем userID из контекста через геттер
	userID := middleware.GetUserID(ctx)

	// Преобразуем gRPC-запрос в нашу внутреннюю модель
	urlModel := model.URLModel{
		OriginalURL: req.GetUrl(),
		UserID:      userID,
	}

	// Сохраняем через сервис (общий слой)
	saved, err := s.svc.SaveURL(ctx, urlModel)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Формируем полный короткий URL
	fullURL, _ := url.JoinPath(s.cfg.BaseURL, saved.ShortURL)

	resp := &shortener.URLShortenResponse{}
	resp.SetResult(fullURL)

	return resp, nil
}

// ExpandURL реализует rpc ExpandURL (аналог GET /{id}).
//
// Логика:
//  1. Проверяем наличие id
//  2. Получаем оригинальный URL через сервис
//  3. Возвращаем через SetResult
//
// Возможные ошибки:
//   - InvalidArgument — id пустой
//   - NotFound       — ссылка не найдена
func (s *shortenerServer) ExpandURL(ctx context.Context, req *shortener.URLExpandRequest) (*shortener.URLExpandResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Получаем оригинальный URL через общий сервис
	urlModel, err := s.svc.GetURL(ctx, req.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "url not found")
	}

	resp := &shortener.URLExpandResponse{}
	resp.SetResult(urlModel.OriginalURL)

	return resp, nil
}

// ListUserURLs реализует rpc ListUserURLs (аналог GET /api/user/urls).
//
// Логика:
//  1. Извлекаем userID из контекста
//  2. Если userID == Nil — возвращаем пустой ответ (не ошибка)
//  3. Получаем список URL пользователя через сервис
//  4. Преобразуем в []*proto.URLData с использованием Set* методов
//
// Возможные ошибки:
//   - Internal — ошибка в GetURLsByUser
func (s *shortenerServer) ListUserURLs(ctx context.Context, req *shortener.ListUserURLsRequest) (*shortener.UserURLsResponse, error) {

	// Извлекаем userID из контекста через геттер
	userID := middleware.GetUserID(ctx)

	// Если userID не найден — возвращаем пустой список
	if userID == uuid.Nil {
		return &shortener.UserURLsResponse{}, nil
	}

	// Получаем список URL пользователя через сервис
	urls, err := s.svc.GetURLsByUser(ctx, userID)
	if err != nil {
		s.logger.Error("GetURLsByUser failed in gRPC", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal server error")
	}

	resp := &shortener.UserURLsResponse{}

	// Преобразуем в gRPC-ответ
	var protoURLs []*shortener.URLData
	for _, u := range urls {
		data := &shortener.URLData{}
		data.SetShortUrl(u.ShortURL)
		data.SetOriginalUrl(u.OriginalURL)
		protoURLs = append(protoURLs, data)
	}

	resp.SetUrls(protoURLs)
	return resp, nil
}
