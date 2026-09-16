package user

import (
	"context"
	"fmt"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

type ListInput struct {
	Limit  int
	Offset int
}

func (input ListInput) Validate() error {
	if input.Limit < 1 || input.Limit > MaxListLimit || input.Offset < 0 {
		return ErrInvalidPagination
	}
	return nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	users, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("listar usuários: %w", err)
	}
	return users, nil
}
