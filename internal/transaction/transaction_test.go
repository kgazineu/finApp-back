package transaction_test

import (
	"errors"
	"strings"
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
	if got.AmountMinor != 12345 || got.NecessityLevel != 5 || got.Kind != "expense" ||
		got.Description != "Lançamento" || got.Category != "Outros" || got.PaymentMethod != "" || got.Installments != 1 ||
		got.OccurredAt.Before(before) || got.OccurredAt.After(time.Now().UTC()) {
		t.Errorf("valores/defaults alterados: %+v", got)
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

func TestNewDetailedIncomeAndExpense(t *testing.T) {
	owner := uuid.New()
	occurred := time.Date(2026, 9, 28, 14, 30, 0, 0, time.FixedZone("local", -3*3600))
	for _, input := range []transaction.CreateInput{
		{UserID: owner, AmountMinor: 1500, Kind: "income", Description: "  Salário  ", Category: " Trabalho ", Installments: 1, OccurredAt: occurred},
		{UserID: owner, AmountMinor: 2200, NecessityLevel: 4, Kind: "expense", Description: " Mercado ", Category: " Casa ", PaymentMethod: "card", Installments: 3, OccurredAt: occurred},
	} {
		got, err := transaction.NewDetailed(input)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == uuid.Nil || got.Kind != input.Kind || got.Description == input.Description || got.Category == input.Category ||
			got.PaymentMethod != input.PaymentMethod || got.Installments != input.Installments || !got.OccurredAt.Equal(occurred) || got.OccurredAt.Location() != time.UTC {
			t.Errorf("transação enriquecida incorreta: %+v", got)
		}
	}
}

func TestNewDetailedRejectsInvalidCombinations(t *testing.T) {
	base := transaction.CreateInput{UserID: uuid.New(), AmountMinor: 100, NecessityLevel: 2, Kind: "expense", Description: "Compra", Category: "Casa", PaymentMethod: "card", Installments: 1, OccurredAt: time.Now()}
	for _, tc := range []struct {
		name string
		edit func(*transaction.CreateInput)
		want error
	}{
		{"missing kind with details", func(i *transaction.CreateInput) { i.Kind = "" }, transaction.ErrInvalidKind},
		{"explicit empty kind", func(i *transaction.CreateInput) {
			*i = transaction.CreateInput{UserID: base.UserID, AmountMinor: 100, NecessityLevel: 1, KindProvided: true}
		}, transaction.ErrInvalidKind},
		{"unknown kind", func(i *transaction.CreateInput) { i.Kind = "transfer" }, transaction.ErrInvalidKind},
		{"negative amount", func(i *transaction.CreateInput) { i.AmountMinor = -1 }, transaction.ErrInvalidAmount},
		{"expense necessity zero", func(i *transaction.CreateInput) { i.NecessityLevel = 0 }, transaction.ErrInvalidNecessityLevel},
		{"income necessity nonzero", func(i *transaction.CreateInput) { i.Kind = "income" }, transaction.ErrInvalidNecessityLevel},
		{"income payment", func(i *transaction.CreateInput) { i.Kind = "income"; i.NecessityLevel = 0 }, transaction.ErrInvalidPaymentMethod},
		{"income installments", func(i *transaction.CreateInput) {
			i.Kind = "income"
			i.NecessityLevel = 0
			i.PaymentMethod = ""
			i.Installments = 2
		}, transaction.ErrInvalidInstallments},
		{"missing expense payment", func(i *transaction.CreateInput) { i.PaymentMethod = "" }, transaction.ErrInvalidPaymentMethod},
		{"unknown expense payment", func(i *transaction.CreateInput) { i.PaymentMethod = "cash" }, transaction.ErrInvalidPaymentMethod},
		{"pix installments", func(i *transaction.CreateInput) { i.PaymentMethod = "pix"; i.Installments = 2 }, transaction.ErrInvalidInstallments},
		{"zero installments", func(i *transaction.CreateInput) { i.Installments = 0 }, transaction.ErrInvalidInstallments},
		{"too many installments", func(i *transaction.CreateInput) { i.Installments = 61 }, transaction.ErrInvalidInstallments},
		{"empty description", func(i *transaction.CreateInput) { i.Description = "  " }, transaction.ErrInvalidDescription},
		{"long description", func(i *transaction.CreateInput) { i.Description = strings.Repeat("x", 201) }, transaction.ErrInvalidDescription},
		{"empty category", func(i *transaction.CreateInput) { i.Category = " " }, transaction.ErrInvalidCategory},
		{"long category", func(i *transaction.CreateInput) { i.Category = strings.Repeat("x", 81) }, transaction.ErrInvalidCategory},
		{"missing occurred at", func(i *transaction.CreateInput) { i.OccurredAt = time.Time{} }, transaction.ErrInvalidOccurredAt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.edit(&input)
			got, err := transaction.NewDetailed(input)
			if !errors.Is(err, tc.want) || got != (transaction.Transaction{}) {
				t.Fatalf("got=%+v err=%v want=%v", got, err, tc.want)
			}
		})
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
