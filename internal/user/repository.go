package user

import "context"

type Repository interface {
	Create(ctx context.Context, u User) (User, error)
	List(ctx context.Context, input ListInput) ([]User, error)
}
