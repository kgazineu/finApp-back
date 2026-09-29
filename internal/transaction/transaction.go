package transaction

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidUserID         = errors.New("ID de usuário inválido")
	ErrInvalidAmount         = errors.New("valor deve ser maior que zero")
	ErrInvalidNecessityLevel = errors.New("nível de necessidade deve estar entre 1 e 5")
)

type Transaction struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// AmountMinor represents the amount in the currency's smallest unit, not a floating-point value.
	AmountMinor    int64
	NecessityLevel int
	CreatedAt      time.Time
}

func New(userID uuid.UUID, amountMinor int64, necessityLevel int) (Transaction, error) {
	if userID == uuid.Nil {
		return Transaction{}, ErrInvalidUserID
	}
	if amountMinor <= 0 {
		return Transaction{}, ErrInvalidAmount
	}
	if necessityLevel < 1 || necessityLevel > 5 {
		return Transaction{}, ErrInvalidNecessityLevel
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Transaction{}, err
	}
	return Transaction{
		ID:             id,
		UserID:         userID,
		AmountMinor:    amountMinor,
		NecessityLevel: necessityLevel,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
