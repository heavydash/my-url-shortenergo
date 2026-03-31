package grpc

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/config"
	"github.com/heavydash/my-url-shortenergo/internal/middleware"
	"github.com/heavydash/my-url-shortenergo/internal/model"
	"github.com/heavydash/my-url-shortenergo/internal/service"
	"github.com/heavydash/my-url-shortenergo/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"strings"
)

// shortenerServer реализует protobuf-интерфейс ShortenerServiceServer.
//
// Тип использует общий бизнес-сервис URLService и конфигурацию для формирования
// полных коротких ссылок. Все методы работают через контекст, в котором может
// находиться userID (устанавливается AuthInterceptor).
type shortenerServer struct {
	shortener.UnimplementedShortenerServiceServer

	cfg *config.Config
	svc service.URLService
}

// NewShortenerService создаёт новую реализацию gRPC-сервиса ShortenerServiceServer.
//
// Функция оборачивает переданный URLService и сохраняет конфигурацию для
// конструирования полных URL. Возвращаемый объект регистрируется в gRPC-сервере.
func NewShortenerService(svc service.URLService, cfg *config.Config) shortener.ShortenerServiceServer {
	return &shortenerServer{svc: svc,
		cfg: cfg}
}

// ShortenURL реализует rpc ShortenURL.
//
// Метод принимает URLShortenRequest, проверяет наличие url, извлекает userID
// из контекста (если есть), сохраняет ссылку через общий сервис и возвращает
// полный короткий URL с префиксом из конфигурации BaseURL.
//
// Возможные ошибки:
//   - InvalidArgument — если req.Url пустой,
//   - Internal       — при ошибке в URLService.
func (s *shortenerServer) ShortenURL(ctx context.Context, req *shortener.URLShortenRequest) (*shortener.URLShortenResponse, error) {
	if req.Url == "" {
		return nil, status.Error(codes.InvalidArgument, "url is required")
	}

	// Извлекаем userID из контекста
	userIDVal := ctx.Value(middleware.UserIDKey)
	userID, _ := userIDVal.(uuid.UUID) // если нет — будет uuid.Nil, сервис сам обработает

	// Преобразуем gRPC-запрос в нашу внутреннюю модель
	urlModel := model.URLModel{
		OriginalURL: req.Url,
		UserID:      userID,
	}

	// Сохраняем через сервис (общий слой)
	saved, err := s.svc.SaveURL(ctx, urlModel)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Формируем полный короткий URL с BaseURL из конфига
	fullURL := fmt.Sprintf("%s/%s",
		strings.TrimRight(s.cfg.BaseURL, "/"),
		saved.ShortURL)

	return &shortener.URLShortenResponse{
		Result: fullURL,
	}, nil
}

// ExpandURL реализует rpc ExpandURL.
//
// Метод возвращает оригинальный URL по короткому идентификатору.
// При отсутствии ссылки возвращает статус NotFound.
func (s *shortenerServer) ExpandURL(ctx context.Context, req *shortener.URLExpandRequest) (*shortener.URLExpandResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Получаем оригинальный URL через общий сервис
	urlModel, err := s.svc.GetURL(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "url not found")
	}

	return &shortener.URLExpandResponse{
		Result: urlModel.OriginalURL,
	}, nil
}

// ListUserURLs реализует rpc ListUserURLs.
//
// Метод возвращает список всех ссылок текущего пользователя.
// Если userID отсутствует в контексте или равен uuid.Nil — возвращается пустой список.
// Не требует дополнительных параметров (принимает emptypb.Empty).
func (s *shortenerServer) ListUserURLs(ctx context.Context, req *emptypb.Empty) (*shortener.UserURLsResponse, error) {

	// Извлекаем userID из контекста
	userIDVal := ctx.Value(middleware.UserIDKey)
	userID, ok := userIDVal.(uuid.UUID)
	if !ok || userID == uuid.Nil {
		// // Если userID нет, возвращаем пустой список
		return &shortener.UserURLsResponse{
			Urls: []*shortener.URLData{},
		}, nil
	}

	// Получаем список URL пользователя через сервис
	urls, err := s.svc.GetURLsByUser(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Преобразуем в gRPC-ответ
	var protoURLs []*shortener.URLData
	for _, u := range urls {
		protoURLs = append(protoURLs, &shortener.URLData{
			ShortUrl:    u.ShortURL,
			OriginalUrl: u.OriginalURL,
		})
	}

	return &shortener.UserURLsResponse{
		Urls: protoURLs,
	}, nil
}
