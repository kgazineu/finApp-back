package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/overview"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type overviewStub func(context.Context, uuid.UUID, time.Time) (overview.Snapshot, error)

func (stub overviewStub) Get(ctx context.Context, owner uuid.UUID, month time.Time) (overview.Snapshot, error) {
	return stub(ctx, owner, month)
}

func dashboardRequest(router http.Handler, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestDashboardRequiresSession(t *testing.T) {
	calls := 0
	session := financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) {
		calls++
		return uuid.Nil, auth.ErrInvalidSession
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(session, nil), api.WithOverviewService(overviewStub(func(context.Context, uuid.UUID, time.Time) (overview.Snapshot, error) {
		t.Fatal("overview called without valid session")
		return overview.Snapshot{}, nil
	}))))
	for _, tc := range []struct {
		token string
		calls int
	}{{"", 0}, {"Basic abc", 0}, {"Bearer invalid", 1}} {
		response := dashboardRequest(router, "/dashboard", tc.token)
		assertGoalError(t, response, http.StatusUnauthorized)
		if calls != tc.calls {
			t.Fatalf("Authenticate called %d times, want %d", calls, tc.calls)
		}
	}
}

func TestDashboardValidatesMonth(t *testing.T) {
	owner := uuid.New()
	calls := 0
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return owner, nil }}, nil), api.WithOverviewService(overviewStub(func(context.Context, uuid.UUID, time.Time) (overview.Snapshot, error) {
		calls++
		return overview.Snapshot{}, nil
	}))))
	for _, month := range []string{"?month=", "?month=2026-00", "?month=2026-13", "?month=0001-01", "?month=0000-01", "?month=2026-9", "?month=2026-09-01", "?month=abc", "?month=2026-09&month=2026-10", "?month=2026-09%20"} {
		response := dashboardRequest(router, "/dashboard"+month, "Bearer valid")
		assertGoalError(t, response, http.StatusBadRequest)
	}
	if calls != 0 {
		t.Fatalf("overview called %d times on invalid input", calls)
	}
}

func TestDashboardResponseAndErrors(t *testing.T) {
	owner := uuid.New()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	id := uuid.New()
	var requested time.Time
	var failure error
	var empty bool
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return owner, nil }}, nil), api.WithOverviewService(overviewStub(func(_ context.Context, gotOwner uuid.UUID, month time.Time) (overview.Snapshot, error) {
		if gotOwner != owner {
			t.Errorf("owner %s, want %s", gotOwner, owner)
		}
		requested = month
		if failure != nil {
			return overview.Snapshot{}, failure
		}
		if empty {
			return overview.Snapshot{Month: start}, nil
		}
		return overview.Snapshot{Month: start, IncomeMinor: 2000, ExpenseMinor: 500, NetTrackedMinor: 1500, GoalsSavedMinor: 200, GoalsTargetMinor: 5000,
			RecentTransactions: []transaction.Transaction{{ID: id, UserID: owner, AmountMinor: 2000, Kind: "income", Description: "Salary", Category: "Work", Installments: 1, OccurredAt: start, CreatedAt: start}},
			ExpensesByCategory: []overview.CategoryTotal{{Category: "Food", TotalMinor: 500}}}, nil
	}))))
	response := dashboardRequest(router, "/dashboard?month=2026-09", "Bearer valid")
	if response.Code != http.StatusOK || !requested.Equal(start) {
		t.Fatalf("HTTP %d, requested %s: %s", response.Code, requested, response.Body.String())
	}
	var body api.DashboardResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Month != "2026-09" || body.IncomeMinor != 2000 || body.ExpenseMinor != 500 || body.NetTrackedMinor != 1500 || body.GoalsSavedMinor != 200 || body.GoalsTargetMinor != 5000 || len(body.ExpensesByCategory) != 1 || body.ExpensesByCategory[0].Category != "Food" || len(body.RecentTransactions) != 1 || body.RecentTransactions[0].Id != id || body.RecentTransactions[0].NecessityLevel != nil || body.RecentTransactions[0].PaymentMethod != nil {
		t.Fatalf("unexpected dashboard: %+v", body)
	}
	empty = true
	response = dashboardRequest(router, "/dashboard", "Bearer valid")
	if response.Code != http.StatusOK || requested.UTC().Format("2006-01") != time.Now().UTC().Format("2006-01") || !strings.Contains(response.Body.String(), `"recentTransactions":[]`) || !strings.Contains(response.Body.String(), `"expensesByCategory":[]`) {
		t.Fatalf("empty/current dashboard HTTP %d: %s", response.Code, response.Body.String())
	}
	failure = errors.New("private database details")
	response = dashboardRequest(router, "/dashboard?month=2026-09", "Bearer valid")
	assertGoalError(t, response, http.StatusInternalServerError)
	if strings.Contains(response.Body.String(), failure.Error()) {
		t.Fatal("internal error details leaked")
	}
}
