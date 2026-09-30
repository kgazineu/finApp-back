package goal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidUserID     = errors.New("invalid user ID")
	ErrInvalidName       = errors.New("goal name must not be empty")
	ErrInvalidTarget     = errors.New("goal target must be positive")
	ErrInvalidAmount     = errors.New("amount must be positive and must not overflow the balance")
	ErrInvalidPagination = errors.New("limit must be between 0 and 100 and offset must be nonnegative")
	ErrNotFound          = errors.New("goal not found")
	ErrInsufficientFunds = errors.New("insufficient saved funds")
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// Goal tracks a virtual allocation; deposits and withdrawals do not transfer money.
type Goal struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	TargetMinor int64
	SavedMinor  int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateInput struct {
	UserID      uuid.UUID
	Name        string
	TargetMinor int64
}

type ListInput struct {
	UserID uuid.UUID
	Limit  int
	Offset int
}

type Repository interface {
	Create(ctx context.Context, item Goal) (Goal, error)
	List(ctx context.Context, input ListInput) ([]Goal, error)
	Deposit(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (Goal, error)
	Withdraw(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (Goal, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, input CreateInput) (Goal, error) {
	if err := ctx.Err(); err != nil {
		return Goal{}, err
	}
	if input.UserID == uuid.Nil {
		return Goal{}, ErrInvalidUserID
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Goal{}, ErrInvalidName
	}
	if input.TargetMinor <= 0 {
		return Goal{}, ErrInvalidTarget
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Goal{}, err
	}
	item, err := s.repo.Create(ctx, Goal{ID: id, UserID: input.UserID, Name: name, TargetMinor: input.TargetMinor})
	if err != nil {
		return Goal{}, fmt.Errorf("create goal: %w", err)
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Goal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.UserID == uuid.Nil {
		return nil, ErrInvalidUserID
	}
	if input.Limit < 0 || input.Limit > MaxListLimit || input.Offset < 0 {
		return nil, ErrInvalidPagination
	}
	if input.Limit == 0 {
		input.Limit = DefaultListLimit
	}
	items, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	return items, nil
}

func (s *Service) Deposit(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (Goal, error) {
	if err := validateAdjustment(ctx, userID, id, amountMinor); err != nil {
		return Goal{}, err
	}
	item, err := s.repo.Deposit(ctx, userID, id, amountMinor)
	if err != nil {
		return Goal{}, fmt.Errorf("deposit into goal: %w", err)
	}
	return item, nil
}

func (s *Service) Withdraw(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (Goal, error) {
	if err := validateAdjustment(ctx, userID, id, amountMinor); err != nil {
		return Goal{}, err
	}
	item, err := s.repo.Withdraw(ctx, userID, id, amountMinor)
	if err != nil {
		return Goal{}, fmt.Errorf("withdraw from goal: %w", err)
	}
	return item, nil
}

func validateAdjustment(ctx context.Context, userID, id uuid.UUID, amountMinor int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if userID == uuid.Nil {
		return ErrInvalidUserID
	}
	if id == uuid.Nil {
		return ErrNotFound
	}
	if amountMinor <= 0 {
		return ErrInvalidAmount
	}
	return nil
}
