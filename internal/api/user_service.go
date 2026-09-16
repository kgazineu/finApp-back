package api

import (
	"context"

	"github.com/kgazineu/finApp-back/internal/user"
)

type UserService interface {
	Create(ctx context.Context, input user.CreateInput) (user.User, error)
	List(ctx context.Context, input user.ListInput) ([]user.User, error)
}
