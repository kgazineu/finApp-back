package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

func TestListTransactionsRequiresSession(t *testing.T) {
	owner := uuid.New()
	calls := 0
	sessions := financialSessionStub{authenticate: func(_ context.Context, _ string) (uuid.UUID, error) {
		return owner, auth.ErrInvalidSession
	}}
	txs := financialTransactionStub{list: func(context.Context, transaction.ListInput) ([]transaction.Transaction, error) {
		calls++
		return nil, nil
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs)))
	for _, header := range []string{"", "Bearer invalid"} {
		req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized || calls != 0 {
			t.Fatalf("listagem não autorizada: %d, calls=%d", response.Code, calls)
		}
	}
}

func TestListTransactionsScopesPaginationAndResponse(t *testing.T) {
	owner := uuid.New()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	inputs := make([]transaction.ListInput, 0)
	sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
		if token != "abc" {
			t.Errorf("token incorreto: %s", token)
		}
		return owner, nil
	}}
	txs := financialTransactionStub{list: func(_ context.Context, input transaction.ListInput) ([]transaction.Transaction, error) {
		inputs = append(inputs, input)
		if input.Offset == 1 {
			return nil, nil
		}
		return []transaction.Transaction{{ID: id, UserID: owner, AmountMinor: 420, NecessityLevel: 2, Kind: "expense", Description: "Supermercado", Category: "Alimentação", PaymentMethod: "pix", Installments: 1, OccurredAt: now, CreatedAt: now}}, nil
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs)))
	for _, tc := range []struct {
		path        string
		wantLimit   int
		wantOffset  int
		wantEntries int
	}{
		{"/transactions", 20, 0, 1},
		{"/transactions?limit=1&offset=1", 1, 1, 0},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer abc")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("status inesperado: %d %s", response.Code, response.Body.String())
		}
		var body struct {
			Data   []map[string]any `json:"data"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data == nil || len(body.Data) != tc.wantEntries || body.Limit != tc.wantLimit || body.Offset != tc.wantOffset {
			t.Fatalf("página incorreta: %+v", body)
		}
		if tc.wantEntries == 1 && (len(body.Data[0]) != 10 || body.Data[0]["id"] != id.String() || body.Data[0]["amountMinor"] != float64(420) || body.Data[0]["description"] != "Supermercado" || body.Data[0]["paymentMethod"] != "pix") {
			t.Errorf("resposta pública incorreta: %+v", body.Data[0])
		}
		got := inputs[len(inputs)-1]
		if got.UserID != owner || got.Limit != tc.wantLimit || got.Offset != tc.wantOffset {
			t.Errorf("identidade ou paginação incorreta: %+v", got)
		}
	}
}

func TestListTransactionsRejectsInvalidPaginationAndHandlesFailure(t *testing.T) {
	calls := 0
	sessions := financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return uuid.New(), nil }}
	txs := financialTransactionStub{list: func(context.Context, transaction.ListInput) ([]transaction.Transaction, error) {
		calls++
		return nil, errors.New("detalhe secreto do banco")
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs)))
	for _, path := range []string{"/transactions?limit=0", "/transactions?limit=101", "/transactions?offset=-1", "/transactions?limit=", "/transactions?limit=1&limit=2", "/transactions?offset=abc"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer abc")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest || calls != 0 {
			t.Fatalf("paginação inválida aceita: %s => %d calls=%d", path, response.Code, calls)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	req.Header.Set("Authorization", "Bearer abc")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || calls != 1 || response.Body.String() == "" {
		t.Fatalf("falha de banco não propagada: %d %s", response.Code, response.Body.String())
	}
}

func TestListDetailedTransactionsResponseAndOwner(t *testing.T) {
	owner := uuid.New()
	otherOwner := uuid.New()
	occurredAt := time.Date(2026, 9, 30, 15, 30, 0, 0, time.UTC)
	incomeID, expenseID := uuid.New(), uuid.New()
	items := []transaction.Transaction{
		{ID: incomeID, UserID: owner, AmountMinor: 5000, Kind: "income", Description: "Salário", Category: "Trabalho", Installments: 1, OccurredAt: occurredAt, CreatedAt: occurredAt},
		{ID: expenseID, UserID: owner, AmountMinor: 1200, Kind: "expense", Description: "Compra", Category: "Casa", PaymentMethod: "debit", Installments: 1, NecessityLevel: 4, OccurredAt: occurredAt, CreatedAt: occurredAt},
	}
	calls := 0
	sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
		if token == "other" {
			return otherOwner, nil
		}
		return owner, nil
	}}
	txs := financialTransactionStub{list: func(_ context.Context, input transaction.ListInput) ([]transaction.Transaction, error) {
		calls++
		if input.Limit != 20 || input.Offset != 0 {
			t.Errorf("unexpected pagination: %+v", input)
		}
		if input.UserID == otherOwner {
			return nil, nil
		}
		if input.UserID != owner {
			t.Errorf("wrong owner: %+v", input)
		}
		return items, nil
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs)))
	for _, tc := range []struct {
		token string
		want  int
	}{{"first", 2}, {"other", 0}} {
		request := httptest.NewRequest(http.MethodGet, "/transactions", nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d: %s", response.Code, response.Body.String())
		}
		var page struct {
			Data   []map[string]any `json:"data"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Data == nil || len(page.Data) != tc.want || page.Limit != 20 || page.Offset != 0 {
			t.Fatalf("unexpected page for %s: %+v", tc.token, page)
		}
		if tc.want == 0 {
			continue
		}
		income, expense := page.Data[0], page.Data[1]
		if len(income) != 8 || income["id"] != incomeID.String() || income["kind"] != "income" || income["amountMinor"] != float64(5000) || income["description"] != "Salário" || income["category"] != "Trabalho" || income["installments"] != float64(1) || income["occurredAt"] != occurredAt.Format(time.RFC3339) || income["createdAt"] != occurredAt.Format(time.RFC3339) {
			t.Errorf("unexpected income: %+v", income)
		}
		if _, ok := income["necessityLevel"]; ok {
			t.Errorf("income exposed necessityLevel: %+v", income)
		}
		if _, ok := income["paymentMethod"]; ok {
			t.Errorf("income exposed paymentMethod: %+v", income)
		}
		if len(expense) != 10 || expense["id"] != expenseID.String() || expense["kind"] != "expense" || expense["amountMinor"] != float64(1200) || expense["description"] != "Compra" || expense["category"] != "Casa" || expense["paymentMethod"] != "debit" || expense["installments"] != float64(1) || expense["necessityLevel"] != float64(4) || expense["occurredAt"] != occurredAt.Format(time.RFC3339) || expense["createdAt"] != occurredAt.Format(time.RFC3339) {
			t.Errorf("unexpected expense: %+v", expense)
		}
		for _, item := range page.Data {
			if _, ok := item["userId"]; ok {
				t.Errorf("owner ID exposed: %+v", item)
			}
		}
	}
	if calls != 2 {
		t.Errorf("List called %d times, want 2", calls)
	}
}
