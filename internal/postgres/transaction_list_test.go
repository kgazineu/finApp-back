package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

func TestTransactionRepositoryListIsScopedAndStable(t *testing.T) {
	db := newTestDB(t)
	first := transactionOwner(t, db)
	second := transactionOwner(t, db)
	repo := postgres.NewTransactionRepository(db)
	now := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	items := []transaction.Transaction{
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), UserID: first, AmountMinor: 100, NecessityLevel: 1, CreatedAt: now},
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000002"), UserID: first, AmountMinor: 200, NecessityLevel: 2, CreatedAt: now.Add(time.Minute)},
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000003"), UserID: first, AmountMinor: 300, NecessityLevel: 3, CreatedAt: now.Add(time.Minute)},
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000004"), UserID: second, AmountMinor: 400, NecessityLevel: 4, CreatedAt: now.Add(time.Minute)},
	}
	expectedAmounts := make(map[uuid.UUID]int64, len(items))
	for _, tx := range items {
		expectedAmounts[tx.ID] = tx.AmountMinor
		if _, err := repo.Create(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		input transaction.ListInput
		want  []uuid.UUID
	}{
		{transaction.ListInput{UserID: first, Limit: 2}, []uuid.UUID{items[2].ID, items[1].ID}},
		{transaction.ListInput{UserID: first, Limit: 2, Offset: 2}, []uuid.UUID{items[0].ID}},
		{transaction.ListInput{UserID: first, Limit: 2, Offset: 3}, nil},
		{transaction.ListInput{UserID: second, Limit: 20}, []uuid.UUID{items[3].ID}},
	} {
		got, err := repo.List(context.Background(), tc.input)
		if err != nil || len(got) != len(tc.want) {
			t.Fatalf("input=%+v: got=%+v err=%v", tc.input, got, err)
		}
		for i, row := range got {
			if row.ID != tc.want[i] || row.UserID != tc.input.UserID || row.AmountMinor != expectedAmounts[row.ID] {
				t.Fatalf("consulta vazou dados ou ordenou incorretamente: %+v", got)
			}
		}
	}
}

func TestTransactionRepositoryListPropagatesDatabaseErrors(t *testing.T) {
	db := newTestDB(t)
	if err := db.Exec("ALTER TABLE transactions RENAME TO transactions_unavailable").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewTransactionRepository(db).List(context.Background(), transaction.ListInput{UserID: uuid.New(), Limit: 20}); err == nil {
		t.Fatal("consulta falha não deve parecer lista vazia")
	}
}

func TestTransactionRepositoryListHonorsCanceledContext(t *testing.T) {
	db, _ := testutil.Database(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := postgres.NewTransactionRepository(db).List(ctx, transaction.ListInput{UserID: uuid.New(), Limit: 20})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado ignorado: %v", err)
	}
}
