package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
)

type GoalService interface {
	Create(ctx context.Context, input goal.CreateInput) (goal.Goal, error)
	List(ctx context.Context, input goal.ListInput) ([]goal.Goal, error)
	Deposit(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (goal.Goal, error)
	Withdraw(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (goal.Goal, error)
}
