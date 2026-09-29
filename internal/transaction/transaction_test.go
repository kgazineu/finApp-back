package transaction_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

func TestNewCreatesTransaction(t *testing.T) {
	ownerID := uuid.New()
	before := time.Now().UTC()
	got, err := transaction.New(ownerID, 12345, 5)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.ID == uuid.Nil || got.UserID != ownerID {
		t.Errorf("identificação inválida: %+v", got)
	}
	if got.AmountMinor != 12345 || got.NecessityLevel != 5 {
		t.Errorf("valores alterados: %+v", got)
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(time.Now().UTC()) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("data de criação inválida: %v", got.CreatedAt)
	}
}

func TestNewAcceptsNecessityBounds(t *testing.T) {
	for _, level := range []int{1, 5} {
		if _, err := transaction.New(uuid.New(), 1, level); err != nil {
			t.Errorf("nível %d deveria ser aceito: %v", level, err)
		}
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	ownerID := uuid.New()
	for _, tc := range []struct {
		name   string
		userID uuid.UUID
		amount int64
		level  int
		want   error
	}{
		{"titular ausente", uuid.Nil, 1, 1, transaction.ErrInvalidUserID},
		{"valor zero", ownerID, 0, 1, transaction.ErrInvalidAmount},
		{"valor negativo", ownerID, -1, 1, transaction.ErrInvalidAmount},
		{"nível zero", ownerID, 1, 0, transaction.ErrInvalidNecessityLevel},
		{"nível negativo", ownerID, 1, -1, transaction.ErrInvalidNecessityLevel},
		{"nível seis", ownerID, 1, 6, transaction.ErrInvalidNecessityLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := transaction.New(tc.userID, tc.amount, tc.level)
			if !errors.Is(err, tc.want) {
				t.Fatalf("esperado %v, recebido %v", tc.want, err)
			}
			if got != (transaction.Transaction{}) {
				t.Errorf("transação inválida retornada: %+v", got)
			}
		})
	}
}
