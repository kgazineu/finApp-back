package user

import (
	"context"
	"fmt"
)

type CreateInput struct {
	Name     string
	Email    string
	Password string
}

type Service struct {
	repo   Repository
	hasher PasswordHasher
}

func NewService(repo Repository, hasher PasswordHasher) *Service {
	return &Service{
		repo:   repo,
		hasher: hasher,
	}
}

func (s *Service) Create(
	ctx context.Context,
	input CreateInput,
) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	if err := validateProfile(input.Name, input.Email); err != nil {
		return User{}, err
	}
	if err := validatePassword(input.Password); err != nil {
		return User{}, err
	}

	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return User{}, fmt.Errorf("gerar hash da senha: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return User{}, err
	}

	u, err := New(input.Name, input.Email, passwordHash)
	if err != nil {
		return User{}, err
	}

	return s.repo.Create(ctx, u)
}
