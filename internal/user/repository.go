package user

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, u User) (User, error)
	List(ctx context.Context, input ListInput) ([]User, error)
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
	Update(ctx context.Context, u User) (User, error)
}
