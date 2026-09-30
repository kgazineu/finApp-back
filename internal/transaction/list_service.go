package transaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

var ErrInvalidPagination = errors.New("limit deve estar entre 1 e 100 e offset deve ser maior ou igual a zero")

type ListInput struct {
	UserID uuid.UUID
	Limit  int
	Offset int
}

func (input ListInput) Validate() error {
	if input.UserID == uuid.Nil {
		return ErrInvalidUserID
	}
	if input.Limit < 1 || input.Limit > MaxListLimit || input.Offset < 0 {
		return ErrInvalidPagination
	}
	return nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("listar transações: %w", err)
	}
	return items, nil
}
