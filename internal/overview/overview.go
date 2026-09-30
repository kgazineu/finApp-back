// Package overview provides a read-only dashboard snapshot of tracked transactions and goals.
package overview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

var (
	ErrInvalidUserID = errors.New("invalid user ID")
	ErrInvalidMonth  = errors.New("month must not be zero")
)

type CategoryTotal struct {
	Category   string
	TotalMinor int64
}

type Snapshot struct {
	Month        time.Time
	IncomeMinor  int64
	ExpenseMinor int64
	// NetTrackedMinor is all-time tracked income minus expense, not a bank balance.
	NetTrackedMinor  int64
	GoalsSavedMinor  int64
	GoalsTargetMinor int64
	// RecentTransactions contains up to five transactions from Month, newest first.
	RecentTransactions []transaction.Transaction
	ExpensesByCategory []CategoryTotal
}

type Repository interface {
	Fetch(ctx context.Context, userID uuid.UUID, start, end time.Time) (Snapshot, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Get(ctx context.Context, userID uuid.UUID, month time.Time) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if userID == uuid.Nil {
		return Snapshot{}, ErrInvalidUserID
	}
	if month.IsZero() {
		return Snapshot{}, ErrInvalidMonth
	}
	utc := month.UTC()
	start := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	result, err := s.repo.Fetch(ctx, userID, start, start.AddDate(0, 1, 0))
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch overview: %w", err)
	}
	result.Month = start
	return result, nil
}
