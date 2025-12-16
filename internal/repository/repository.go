package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/heavydash/my-url-shortenergo/internal/model"
)

//go:generate mockgen -source=$GOFILE -destination=../repository/mocks/mock_repository.go -package=mocks
type URLRepository interface {
	SaveURL(m model.URLModel) (model.URLModel, error)
	GetURL(id string) (model.URLModel, error)
	SaveBatch(ctx context.Context, batch []model.URLModel) error
	Clear() error
	Ping(ctx context.Context) error
	GetURLsByUser(ctx context.Context, userID uuid.UUID) ([]model.URLModel, error)
	MarkAsDeleted(userID uuid.UUID, shortURLs []string) error
}
