package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/overview"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestDashboardEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	ctx := context.Background()
	users := postgres.NewUserRepository(db)
	createUser := func() uuid.UUID {
		t.Helper()
		item, err := user.New("Dashboard owner", uuid.NewString()+"@example.com", "hash")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := users.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
		return item.ID
	}
	owner, other := createUser(), createUser()
	month := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	transactions := transaction.NewService(postgres.NewTransactionRepository(db))
	for _, input := range []transaction.CreateInput{
		{UserID: owner, Kind: "income", AmountMinor: 2000, Description: "Salary", Category: "Work", Installments: 1, OccurredAt: month},
		{UserID: owner, Kind: "expense", AmountMinor: 350, NecessityLevel: 3, Description: "Food", Category: "Food", PaymentMethod: "pix", Installments: 1, OccurredAt: month.Add(time.Hour)},
		{UserID: other, Kind: "expense", AmountMinor: 9000, NecessityLevel: 3, Description: "Private", Category: "Food", PaymentMethod: "pix", Installments: 1, OccurredAt: month},
	} {
		if _, err := transactions.Create(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	goals := goal.NewService(postgres.NewGoalRepository(db))
	item, err := goals.Create(ctx, goal.CreateInput{UserID: owner, Name: "Emergency", TargetMinor: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goals.Deposit(ctx, owner, item.ID, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := goals.Create(ctx, goal.CreateInput{UserID: other, Name: "Private", TargetMinor: 9000}); err != nil {
		t.Fatal(err)
	}
	session := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
		if token == "owner" {
			return owner, nil
		}
		return uuid.Nil, auth.ErrInvalidSession
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(session, transactions), api.WithOverviewService(overview.NewService(postgres.NewOverviewRepository(db)))))
	response := dashboardRequest(router, "/dashboard?month=2026-09", "Bearer owner")
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard HTTP %d: %s", response.Code, response.Body.String())
	}
	var body api.DashboardResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Month != "2026-09" || body.IncomeMinor != 2000 || body.ExpenseMinor != 350 || body.NetTrackedMinor != 1650 || body.GoalsSavedMinor != 300 || body.GoalsTargetMinor != 5000 || len(body.ExpensesByCategory) != 1 || body.ExpensesByCategory[0].TotalMinor != 350 || len(body.RecentTransactions) != 2 {
		t.Fatalf("dashboard leaked another owner or has incorrect totals: %+v", body)
	}
	response = dashboardRequest(router, "/dashboard?month=2026-10", "Bearer owner")
	if response.Code != http.StatusOK {
		t.Fatalf("next month HTTP %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.IncomeMinor != 0 || body.ExpenseMinor != 0 || body.NetTrackedMinor != 1650 || len(body.RecentTransactions) != 0 {
		t.Fatalf("next month dashboard %+v: %v", body, err)
	}
}
