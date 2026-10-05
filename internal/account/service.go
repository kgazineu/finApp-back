package account

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

var ErrNotFound = errors.New("conta não encontrada")
var ErrLiabilityWithYield = errors.New("conta de passivo não pode ter rendimento")

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, a *Account) (*Account, error) {
	return s.repo.Create(ctx, userID, a)
}

func (s *Service) FindActive(ctx context.Context, userID uuid.UUID) ([]*Account, error) {
	return s.repo.FindActive(ctx, userID)
}

func (s *Service) FindByID(ctx context.Context, userID uuid.UUID, id int64) (*Account, error) {
	return s.repo.FindByID(ctx, userID, id)
}

func (s *Service) Update(ctx context.Context, userID uuid.UUID, a *Account) (*Account, error) {
	account, err := s.repo.FindByID(ctx, userID, a.ID)
	if err != nil {
		return nil, err
	}

	if account.ArchivedAt != nil {
		return nil, ErrNotFound
	}

	// o tipo que vale é o novo: virar liability com rendimento é proibido
	if a.Kind == KindLiability && a.HasYield {
		return nil, ErrLiabilityWithYield
	}

	return s.repo.Update(ctx, userID, a)
}

func (s *Service) Delete(ctx context.Context, userID uuid.UUID, id int64) (*DeleteResult, error) {
	_, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return s.repo.Delete(ctx, userID, id)
}
