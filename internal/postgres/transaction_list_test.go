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

func TestTransactionRepositoryListUsesOccurredAtAndReadsLegacyDefaults(t *testing.T) {
	db := newTestDB(t)
	owner := transactionOwner(t, db)
	repo := postgres.NewTransactionRepository(db)
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	legacyID := uuid.New()
	if err := db.Exec(`INSERT INTO transactions (id, user_id, amount_minor, necessity_level, created_at, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`, legacyID, owner, 100, 3, base, base).Error; err != nil {
		t.Fatal(err)
	}
	income, err := transaction.NewDetailed(transaction.CreateInput{
		UserID: owner, AmountMinor: 500, Kind: "income", Description: "Salário", Category: "Trabalho", Installments: 1, OccurredAt: base.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	income.CreatedAt = base.Add(-time.Hour)
	if _, err := repo.Create(context.Background(), income); err != nil {
		t.Fatal(err)
	}
	laterID := uuid.New()
	later := transaction.Transaction{
		ID: laterID, UserID: owner, AmountMinor: 200, NecessityLevel: 1,
		Kind: "expense", Description: "Compra", Category: "Casa", PaymentMethod: "pix", Installments: 1,
		OccurredAt: base.Add(time.Hour), CreatedAt: base.Add(-2 * time.Hour),
	}
	if _, err := repo.Create(context.Background(), later); err != nil {
		t.Fatal(err)
	}
	items, err := repo.List(context.Background(), transaction.ListInput{UserID: owner, Limit: 20})
	if err != nil || len(items) != 3 {
		t.Fatalf("listagem: %+v err=%v", items, err)
	}
	first, second := income.ID, laterID
	if first.String() < second.String() {
		first, second = second, first
	}
	if items[0].ID != first || items[1].ID != second || items[2].ID != legacyID {
		t.Errorf("ordenação por occurred_at/id incorreta: %+v", items)
	}
	if items[2].Kind != "expense" || items[2].Description != "Lançamento" || items[2].Category != "Outros" ||
		items[2].Installments != 1 || items[2].PaymentMethod != "" || items[2].NecessityLevel != 3 {
		t.Errorf("leitura da linha legada incorreta: %+v", items[2])
	}
	if items[0].Kind == "income" && items[0].NecessityLevel != 0 || items[1].Kind == "income" && items[1].NecessityLevel != 0 {
		t.Errorf("income deveria ter necessidade zero: %+v", items)
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
