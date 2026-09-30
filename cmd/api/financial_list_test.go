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
		return []transaction.Transaction{{ID: id, UserID: owner, AmountMinor: 420, NecessityLevel: 2, CreatedAt: now}}, nil
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
		if tc.wantEntries == 1 && (len(body.Data[0]) != 4 || body.Data[0]["id"] != id.String() || body.Data[0]["amountMinor"] != float64(420)) {
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
