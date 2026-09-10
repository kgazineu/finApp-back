package api

import (
	"context"

	"github.com/kgazineu/finApp-back/internal/user"
)

type UserCreator interface {
	Create(ctx context.Context, input user.CreateInput) (user.User, error)
}
