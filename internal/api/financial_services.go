package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type SessionService interface {
	Login(ctx context.Context, email, password string) (string, time.Time, error)
	Authenticate(ctx context.Context, token string) (uuid.UUID, error)
}

type TransactionService interface {
	Create(ctx context.Context, input transaction.CreateInput) (transaction.Transaction, error)
	List(ctx context.Context, input transaction.ListInput) ([]transaction.Transaction, error)
}
