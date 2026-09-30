package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

type UserService interface {
	Create(ctx context.Context, input user.CreateInput) (user.User, error)
	List(ctx context.Context, input user.ListInput) ([]user.User, error)
	Get(ctx context.Context, id uuid.UUID) (user.User, error)
	Update(ctx context.Context, input user.UpdateInput) (user.User, error)
}
