package postgres_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/overview"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

func overviewOwner(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	owner, err := user.New("Overview owner", uuid.NewString()+"@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewUserRepository(db).Create(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	return owner.ID
}

func overviewTransaction(t *testing.T, db *gorm.DB, owner uuid.UUID, kind, category string, amount int64, at time.Time) transaction.Transaction {
	t.Helper()
	input := transaction.CreateInput{
		UserID: owner, AmountMinor: amount, Kind: kind, Description: "Recorded item",
		Category: category, Installments: 1, OccurredAt: at,
	}
	if kind == "expense" {
		input.NecessityLevel = 2
		input.PaymentMethod = "pix"
	}
	tx, err := transaction.NewDetailed(input)
	if err != nil {
		t.Fatal(err)
	}
	tx, err = postgres.NewTransactionRepository(db).Create(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func overviewGoal(t *testing.T, db *gorm.DB, owner uuid.UUID, target, saved int64) {
	t.Helper()
	service := goal.NewService(postgres.NewGoalRepository(db))
	item, err := service.Create(context.Background(), goal.CreateInput{UserID: owner, Name: uuid.NewString(), TargetMinor: target})
	if err != nil {
		t.Fatal(err)
	}
	if saved > 0 {
		if _, err := service.Deposit(context.Background(), owner, item.ID, saved); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOverviewEmptyAndCanceled(t *testing.T) {
	db, _ := testutil.Database(t)
	owner := overviewOwner(t, db)
	service := overview.NewService(postgres.NewOverviewRepository(db))
	month := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	got, err := service.Get(context.Background(), owner, month)
	if err != nil {
		t.Fatal(err)
	}
	if got.Month != month || got.IncomeMinor != 0 || got.ExpenseMinor != 0 || got.NetTrackedMinor != 0 ||
		got.GoalsSavedMinor != 0 || got.GoalsTargetMinor != 0 || len(got.RecentTransactions) != 0 || len(got.ExpensesByCategory) != 0 {
		t.Fatalf("unexpected empty snapshot: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := postgres.NewOverviewRepository(db).Fetch(ctx, owner, month, month.AddDate(0, 1, 0)); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled query, got %v", err)
	}
}

func TestOverviewMonthOwnershipAndRecent(t *testing.T) {
	db, _ := testutil.Database(t)
	owner, other := overviewOwner(t, db), overviewOwner(t, db)
	service := overview.NewService(postgres.NewOverviewRepository(db))
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	overviewTransaction(t, db, owner, "expense", "Outside", 7, start.Add(-time.Microsecond))
	overviewTransaction(t, db, owner, "income", "Outside", 900, end)
	entries := []transaction.Transaction{
		overviewTransaction(t, db, owner, "expense", "Food", 200, start),
		overviewTransaction(t, db, owner, "income", "Salary", 1000, start.Add(time.Hour)),
		overviewTransaction(t, db, owner, "expense", "Food", 100, start.Add(2*time.Hour)),
		overviewTransaction(t, db, owner, "income", "Gift", 50, start.Add(3*time.Hour)),
		overviewTransaction(t, db, owner, "expense", "Rent", 50, start.Add(4*time.Hour)),
		overviewTransaction(t, db, owner, "expense", "Rent", 25, start.Add(5*time.Hour)),
		overviewTransaction(t, db, owner, "expense", "Alpha", 10, end.Add(-time.Microsecond)),
	}
	overviewTransaction(t, db, other, "income", "Private", 999, start)
	overviewTransaction(t, db, other, "expense", "Food", 999, start)
	overviewGoal(t, db, owner, 500, 80)
	overviewGoal(t, db, owner, 200, 20)
	overviewGoal(t, db, other, 999, 999)

	got, err := service.Get(context.Background(), owner, start.Add(14*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.Month != start || got.IncomeMinor != 1050 || got.ExpenseMinor != 385 ||
		got.NetTrackedMinor != 1558 || got.GoalsSavedMinor != 100 || got.GoalsTargetMinor != 700 {
		t.Fatalf("incorrect totals or leaked owner data: %+v", got)
	}
	wantCategories := []overview.CategoryTotal{{Category: "Food", TotalMinor: 300}, {Category: "Rent", TotalMinor: 75}, {Category: "Alpha", TotalMinor: 10}}
	if !reflect.DeepEqual(got.ExpensesByCategory, wantCategories) {
		t.Fatalf("categories: got=%+v want=%+v", got.ExpensesByCategory, wantCategories)
	}
	if len(got.RecentTransactions) != 5 {
		t.Fatalf("expected 5 monthly recent transactions, got %+v", got.RecentTransactions)
	}
	for i, tx := range got.RecentTransactions {
		want := entries[len(entries)-1-i]
		if tx.ID != want.ID || tx.UserID != owner || tx.Kind != want.Kind || tx.AmountMinor != want.AmountMinor ||
			tx.Description != want.Description || tx.Category != want.Category || !tx.OccurredAt.Equal(want.OccurredAt) {
			t.Fatalf("recent[%d]: got=%+v want=%+v", i, tx, want)
		}
	}
	// Monthly values change at the boundary; tracked net and goals remain all-time.
	april, err := service.Get(context.Background(), owner, end)
	if err != nil || april.IncomeMinor != 900 || april.ExpenseMinor != 0 || len(april.RecentTransactions) != 1 ||
		april.NetTrackedMinor != 1558 || april.GoalsSavedMinor != 100 {
		t.Fatalf("next month: %+v err=%v", april, err)
	}
}

func TestOverviewCategoryLimitAndTieOrder(t *testing.T) {
	db, _ := testutil.Database(t)
	owner := overviewOwner(t, db)
	start := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"L", "K", "J", "I", "H", "G", "F", "E", "D", "C", "B", "A"} {
		overviewTransaction(t, db, owner, "expense", name, 1, start)
	}
	got, err := overview.NewService(postgres.NewOverviewRepository(db)).Get(context.Background(), owner, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ExpensesByCategory) != 10 || got.ExpenseMinor != 12 {
		t.Fatalf("unexpected category limit: %+v", got)
	}
	for i, item := range got.ExpensesByCategory {
		if item.Category != string(rune('A'+i)) || item.TotalMinor != 1 {
			t.Fatalf("category[%d] = %+v", i, item)
		}
	}
}

func TestOverviewRejectsOverflow(t *testing.T) {
	for _, scenario := range []string{"monthly", "net", "goals"} {
		t.Run(scenario, func(t *testing.T) {
			db, _ := testutil.Database(t)
			owner := overviewOwner(t, db)
			start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
			switch scenario {
			case "monthly":
				overviewTransaction(t, db, owner, "expense", "One", math.MaxInt64, start)
				overviewTransaction(t, db, owner, "expense", "Two", 1, start)
			case "net":
				overviewTransaction(t, db, owner, "expense", "Old", math.MaxInt64, start.AddDate(0, -1, 0))
				overviewTransaction(t, db, owner, "expense", "Old", 2, start.AddDate(0, -1, 0))
			case "goals":
				overviewGoal(t, db, owner, math.MaxInt64, math.MaxInt64)
				overviewGoal(t, db, owner, 1, 1)
			}
			if _, err := overview.NewService(postgres.NewOverviewRepository(db)).Get(context.Background(), owner, start); err == nil {
				t.Fatal("expected error for total exceeding int64")
			}
		})
	}
}
