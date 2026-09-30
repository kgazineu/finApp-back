package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type financialSessionStub struct {
	login        func(context.Context, string, string) (string, time.Time, error)
	authenticate func(context.Context, string) (uuid.UUID, error)
}

func (s financialSessionStub) Login(ctx context.Context, email, password string) (string, time.Time, error) {
	if s.login == nil {
		panic("unexpected Login call")
	}
	return s.login(ctx, email, password)
}

func (s financialSessionStub) Authenticate(ctx context.Context, token string) (uuid.UUID, error) {
	if s.authenticate == nil {
		panic("unexpected Authenticate call")
	}
	return s.authenticate(ctx, token)
}

type financialTransactionStub struct {
	create func(context.Context, transaction.CreateInput) (transaction.Transaction, error)
	list   func(context.Context, transaction.ListInput) ([]transaction.Transaction, error)
}

func (s financialTransactionStub) Create(ctx context.Context, input transaction.CreateInput) (transaction.Transaction, error) {
	if s.create == nil {
		panic("unexpected Create call")
	}
	return s.create(ctx, input)
}

func (s financialTransactionStub) List(ctx context.Context, input transaction.ListInput) ([]transaction.Transaction, error) {
	if s.list == nil {
		panic("unexpected List call")
	}
	return s.list(ctx, input)
}

func financialRequest(t *testing.T, router http.Handler, path, body, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestFinancialSessionHandler(t *testing.T) {
	expires := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	const secret = "session-secret-should-not-appear-in-errors"
	for _, tt := range []struct {
		name, body string
		loginErr   error
		status     int
		calls      int
	}{
		{"valid", `{"email":"alice@example.com","password":"correct"}`, nil, http.StatusOK, 1},
		{"invalid credentials", `{"email":"alice@example.com","password":"wrong"}`, fmt.Errorf("login: %w", auth.ErrInvalidCredentials), http.StatusUnauthorized, 1},
		{"internal error", `{"email":"alice@example.com","password":"correct"}`, errors.New(secret), http.StatusInternalServerError, 1},
		{"malformed JSON", `{`, nil, http.StatusBadRequest, 0},
		{"non-string password", `{"email":"alice@example.com","password":123}`, nil, http.StatusBadRequest, 0},
		{"missing password", `{"email":"alice@example.com"}`, nil, http.StatusBadRequest, 0},
		{"extra JSON", `{"email":"alice@example.com","password":"correct"} {}`, nil, http.StatusBadRequest, 0},
		{"oversized JSON", `{"email":"` + strings.Repeat("x", 65536) + `","password":"correct"}`, nil, http.StatusRequestEntityTooLarge, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			sessions := financialSessionStub{login: func(_ context.Context, email, password string) (string, time.Time, error) {
				calls++
				if email != "alice@example.com" || (password != "correct" && password != "wrong") {
					t.Errorf("unexpected login input: %q / %q", email, password)
				}
				return secret, expires, tt.loginErr
			}}
			response := financialRequest(t, newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, financialTransactionStub{}))), "/sessions", tt.body, "")
			if response.Code != tt.status || calls != tt.calls {
				t.Fatalf("status %d, Login calls %d; want %d, %d: %s", response.Code, calls, tt.status, tt.calls, response.Body.String())
			}
			if tt.status != http.StatusOK {
				var body api.ErrorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message == "" {
					t.Fatalf("expected JSON error: %s (%v)", response.Body.String(), err)
				}
				if strings.Contains(response.Body.String(), secret) {
					t.Fatal("session secret leaked in error response")
				}
				return
			}
			var body struct {
				Token     string    `json:"token"`
				TokenType string    `json:"tokenType"`
				ExpiresAt time.Time `json:"expiresAt"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Token != secret || body.TokenType != "Bearer" || !body.ExpiresAt.Equal(expires) {
				t.Errorf("unexpected session response: %+v", body)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Error("token response must not be cached")
			}
		})
	}
}

func TestFinancialTransactionHandler(t *testing.T) {
	owner := uuid.New()
	createdID := uuid.New()
	createdAt := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	const secret = "bearer-token-should-not-appear-in-errors"
	const validBody = `{"amountMinor":12345,"necessityLevel":3}`
	for _, tt := range []struct {
		name, authHeader, body string
		authErr                error
		createErr              error
		status, authCalls      int
		createCalls            int
	}{
		{"authorized", "Bearer " + secret, validBody, nil, nil, http.StatusCreated, 1, 1},
		{"missing auth", "", validBody, nil, nil, http.StatusUnauthorized, 0, 0},
		{"invalid token", "Bearer " + secret, validBody, auth.ErrInvalidSession, nil, http.StatusUnauthorized, 1, 0},
		{"expired token", "Bearer " + secret, validBody, fmt.Errorf("expired: %w", auth.ErrInvalidSession), nil, http.StatusUnauthorized, 1, 0},
		{"wrong scheme", "Basic " + secret, validBody, nil, nil, http.StatusUnauthorized, 0, 0},
		{"auth failure", "Bearer " + secret, validBody, errors.New(secret), nil, http.StatusInternalServerError, 1, 0},
		{"zero amount", "Bearer " + secret, `{"amountMinor":0,"necessityLevel":3}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"negative amount", "Bearer " + secret, `{"amountMinor":-1,"necessityLevel":3}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"level below range", "Bearer " + secret, `{"amountMinor":10,"necessityLevel":0}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"level above range", "Bearer " + secret, `{"amountMinor":10,"necessityLevel":6}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"noninteger amount", "Bearer " + secret, `{"amountMinor":1.5,"necessityLevel":3}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"noninteger level", "Bearer " + secret, `{"amountMinor":10,"necessityLevel":"3"}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"int64 overflow", "Bearer " + secret, `{"amountMinor":9223372036854775808,"necessityLevel":3}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"malformed JSON", "Bearer " + secret, `{`, nil, nil, http.StatusBadRequest, 1, 0},
		{"userId not accepted", "Bearer " + secret, `{"amountMinor":10,"necessityLevel":3,"userId":"` + uuid.NewString() + `"}`, nil, nil, http.StatusBadRequest, 1, 0},
		{"oversized body", "Bearer " + secret, `{"amountMinor":10,"necessityLevel":3,"extra":"` + strings.Repeat("x", 65536) + `"}`, nil, nil, http.StatusRequestEntityTooLarge, 1, 0},
		{"create failure", "Bearer " + secret, validBody, nil, errors.New(secret), http.StatusInternalServerError, 1, 1},
		{"invalid service amount", "Bearer " + secret, validBody, nil, transaction.ErrInvalidAmount, http.StatusBadRequest, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authCalls, createCalls := 0, 0
			sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
				authCalls++
				if token != secret {
					t.Errorf("unexpected token passed to Authenticate: %q", token)
				}
				return owner, tt.authErr
			}}
			transactions := financialTransactionStub{create: func(_ context.Context, input transaction.CreateInput) (transaction.Transaction, error) {
				createCalls++
				if input != (transaction.CreateInput{UserID: owner, AmountMinor: 12345, NecessityLevel: 3}) {
					t.Errorf("unexpected Create input: %+v", input)
				}
				return transaction.Transaction{ID: createdID, UserID: owner, AmountMinor: input.AmountMinor, NecessityLevel: input.NecessityLevel, Kind: "expense", Description: "Lançamento", Category: "Outros", Installments: 1, OccurredAt: createdAt, CreatedAt: createdAt}, tt.createErr
			}}
			response := financialRequest(t, newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, transactions))), "/transactions", tt.body, tt.authHeader)
			if response.Code != tt.status || authCalls != tt.authCalls || createCalls != tt.createCalls {
				t.Fatalf("status %d, Authenticate %d, Create %d; want %d, %d, %d: %s", response.Code, authCalls, createCalls, tt.status, tt.authCalls, tt.createCalls, response.Body.String())
			}
			if tt.status != http.StatusCreated {
				var body api.ErrorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message == "" {
					t.Fatalf("expected JSON error: %s (%v)", response.Body.String(), err)
				}
				if strings.Contains(response.Body.String(), secret) {
					t.Fatal("bearer token leaked in error response")
				}
				if tt.status == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Error("missing Bearer challenge")
				}
				return
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 9 || body["id"] != createdID.String() || body["amountMinor"] != float64(12345) || body["necessityLevel"] != float64(3) || body["kind"] != "expense" || body["description"] != "Lançamento" || body["category"] != "Outros" || body["installments"] != float64(1) || body["occurredAt"] != createdAt.Format(time.RFC3339) || body["createdAt"] != createdAt.Format(time.RFC3339) {
				t.Errorf("unexpected public transaction response: %+v", body)
			}
		})
	}
}

func TestCreateDetailedTransactions(t *testing.T) {
	owner := uuid.New()
	occurredAt := time.Date(2026, 9, 30, 15, 30, 0, 0, time.UTC)
	createdAt := occurredAt.Add(time.Hour)
	for _, tc := range []struct {
		name, body string
		input      transaction.CreateInput
		result     transaction.Transaction
		wantFields int
	}{
		{
			name:       "income without expense fields",
			body:       `{"amountMinor":75000,"kind":"income","description":" Salário ","category":" Trabalho ","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`,
			input:      transaction.CreateInput{UserID: owner, AmountMinor: 75000, Kind: "income", KindProvided: true, Description: " Salário ", Category: " Trabalho ", Installments: 1, OccurredAt: occurredAt},
			result:     transaction.Transaction{AmountMinor: 75000, Kind: "income", Description: "Salário", Category: "Trabalho", Installments: 1, OccurredAt: occurredAt, CreatedAt: createdAt},
			wantFields: 8,
		},
		{
			name:       "card expense with installments",
			body:       `{"amountMinor":23599,"kind":"expense","description":"Notebook","category":"Tecnologia","paymentMethod":"card","installments":12,"necessityLevel":2,"occurredAt":"2026-09-30T15:30:00Z"}`,
			input:      transaction.CreateInput{UserID: owner, AmountMinor: 23599, Kind: "expense", KindProvided: true, Description: "Notebook", Category: "Tecnologia", PaymentMethod: "card", Installments: 12, NecessityLevel: 2, OccurredAt: occurredAt},
			result:     transaction.Transaction{AmountMinor: 23599, Kind: "expense", Description: "Notebook", Category: "Tecnologia", PaymentMethod: "card", Installments: 12, NecessityLevel: 2, OccurredAt: occurredAt, CreatedAt: createdAt},
			wantFields: 10,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.New()
			calls := 0
			sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
				if token != "secret" {
					t.Errorf("unexpected token: %q", token)
				}
				return owner, nil
			}}
			txs := financialTransactionStub{create: func(_ context.Context, input transaction.CreateInput) (transaction.Transaction, error) {
				calls++
				if input != tc.input {
					t.Errorf("Create input: %+v, want %+v", input, tc.input)
				}
				result := tc.result
				result.ID, result.UserID = id, owner
				return result, nil
			}}
			response := financialRequest(t, newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs))), "/transactions", tc.body, "Bearer secret")
			if response.Code != http.StatusCreated || calls != 1 {
				t.Fatalf("status=%d Create calls=%d: %s", response.Code, calls, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != tc.wantFields || body["id"] != id.String() || body["amountMinor"] != float64(tc.result.AmountMinor) || body["kind"] != tc.result.Kind || body["description"] != tc.result.Description || body["category"] != tc.result.Category || body["installments"] != float64(tc.result.Installments) || body["occurredAt"] != occurredAt.Format(time.RFC3339) || body["createdAt"] != createdAt.Format(time.RFC3339) {
				t.Errorf("unexpected public response: %+v", body)
			}
			if tc.result.Kind == "income" {
				if _, exists := body["necessityLevel"]; exists {
					t.Error("income must omit necessityLevel")
				}
				if _, exists := body["paymentMethod"]; exists {
					t.Error("income must omit paymentMethod")
				}
			} else if body["necessityLevel"] != float64(2) || body["paymentMethod"] != "card" {
				t.Errorf("missing expense fields: %+v", body)
			}
			if _, exists := body["userId"]; exists {
				t.Error("owner ID exposed in response")
			}
		})
	}
}

func TestCreateDetailedTransactionsRejectsInvalidInput(t *testing.T) {
	const base = `"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":1,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"`
	for _, tc := range []struct{ name, body string }{
		{"missing kind with details", `{"amountMinor":10,"description":"Compra","category":"Casa","paymentMethod":"pix","installments":1,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"empty explicit kind", `{"amountMinor":10,"kind":""}`},
		{"unknown kind", `{"amountMinor":10,"kind":"transfer"}`},
		{"missing description", `{"amountMinor":10,"kind":"income","category":"Salário","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"blank category", `{"amountMinor":10,"kind":"income","description":"Salário","category":"  ","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"description too long", `{"amountMinor":10,"kind":"income","description":"` + strings.Repeat("a", 201) + `","category":"Salário","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"category too long", `{"amountMinor":10,"kind":"income","description":"Salário","category":"` + strings.Repeat("a", 81) + `","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"income necessity", `{"amountMinor":10,"kind":"income","description":"Salário","category":"Trabalho","installments":1,"necessityLevel":2,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"income explicit zero necessity", `{"amountMinor":10,"kind":"income","description":"Salário","category":"Trabalho","installments":1,"necessityLevel":0,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"income payment method", `{"amountMinor":10,"kind":"income","description":"Salário","category":"Trabalho","paymentMethod":"pix","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"income installments", `{"amountMinor":10,"kind":"income","description":"Salário","category":"Trabalho","installments":2,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"expense missing necessity", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":1,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"expense missing payment", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","installments":1,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"invalid payment", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"cash","installments":1,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"pix installments", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":2,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"zero installments", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"card","installments":0,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"too many installments", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"card","installments":61,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`},
		{"missing date", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":1,"necessityLevel":3}`},
		{"invalid date", `{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":1,"necessityLevel":3,"occurredAt":"yesterday"}`},
		{"unknown field", `{` + base + `,"userId":"` + uuid.NewString() + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sessions := financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return uuid.New(), nil }}
			txs := financialTransactionStub{create: func(context.Context, transaction.CreateInput) (transaction.Transaction, error) {
				calls++
				return transaction.Transaction{}, nil
			}}
			response := financialRequest(t, newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, txs))), "/transactions", tc.body, "Bearer secret")
			if response.Code != http.StatusBadRequest || calls != 0 {
				t.Fatalf("status=%d Create calls=%d: %s", response.Code, calls, response.Body.String())
			}
			var body api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message == "" {
				t.Fatalf("expected JSON error: %s (%v)", response.Body.String(), err)
			}
		})
	}
}
