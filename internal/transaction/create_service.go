package transaction

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type CreateInput struct {
	UserID         uuid.UUID
	AmountMinor    int64
	NecessityLevel int
}

type Repository interface {
	Create(ctx context.Context, tx Transaction) (Transaction, error)
	List(ctx context.Context, input ListInput) ([]Transaction, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Transaction, error) {
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	tx, err := New(input.UserID, input.AmountMinor, input.NecessityLevel)
	if err != nil {
		return Transaction{}, err
	}
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	created, err := s.repo.Create(ctx, tx)
	if err != nil {
		return Transaction{}, fmt.Errorf("persistir transação: %w", err)
	}
	return created, nil
}
