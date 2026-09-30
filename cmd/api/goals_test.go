package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/goal"
)

type goalServiceStub struct {
	create   func(context.Context, goal.CreateInput) (goal.Goal, error)
	list     func(context.Context, goal.ListInput) ([]goal.Goal, error)
	deposit  func(context.Context, uuid.UUID, uuid.UUID, int64) (goal.Goal, error)
	withdraw func(context.Context, uuid.UUID, uuid.UUID, int64) (goal.Goal, error)
}

func (s goalServiceStub) Create(ctx context.Context, input goal.CreateInput) (goal.Goal, error) {
	if s.create == nil {
		panic("unexpected goal Create")
	}
	return s.create(ctx, input)
}
func (s goalServiceStub) List(ctx context.Context, input goal.ListInput) ([]goal.Goal, error) {
	if s.list == nil {
		panic("unexpected goal List")
	}
	return s.list(ctx, input)
}
func (s goalServiceStub) Deposit(ctx context.Context, userID, id uuid.UUID, amount int64) (goal.Goal, error) {
	if s.deposit == nil {
		panic("unexpected goal Deposit")
	}
	return s.deposit(ctx, userID, id, amount)
}
func (s goalServiceStub) Withdraw(ctx context.Context, userID, id uuid.UUID, amount int64) (goal.Goal, error) {
	if s.withdraw == nil {
		panic("unexpected goal Withdraw")
	}
	return s.withdraw(ctx, userID, id, amount)
}

func goalRequest(t *testing.T, router http.Handler, method, path, body, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func assertGoalError(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", response.Code, status, response.Body.String())
	}
	var body api.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message == "" {
		t.Fatalf("expected JSON error: %s (%v)", response.Body.String(), err)
	}
	if status == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Error("missing Bearer challenge")
	}
}

func assertGoalBody(t *testing.T, response *httptest.ResponseRecorder, status int, item goal.Goal) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", response.Code, status, response.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": item.ID.String(), "name": item.Name, "targetMinor": float64(item.TargetMinor),
		"savedMinor": float64(item.SavedMinor), "createdAt": item.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt": item.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("public goal response = %v, want %v", got, want)
	}
}

func TestGoalRoutesRequireSession(t *testing.T) {
	id := uuid.NewString()
	for _, route := range []struct{ method, path, body string }{
		{http.MethodPost, "/goals", `{"name":"Trip","targetMinor":100}`},
		{http.MethodGet, "/goals", ""},
		{http.MethodPost, "/goals/" + id + "/allocations", `{"direction":"deposit","amountMinor":10}`},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			calls := 0
			sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
				calls++
				if token != "invalid" {
					t.Errorf("unexpected token %q", token)
				}
				return uuid.Nil, auth.ErrInvalidSession
			}}
			router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, financialTransactionStub{}), api.WithGoalService(goalServiceStub{})))
			for _, tc := range []struct {
				header string
				calls  int
			}{{"", 0}, {"Basic invalid", 0}, {"Bearer invalid", 1}} {
				response := goalRequest(t, router, route.method, route.path, route.body, tc.header)
				assertGoalError(t, response, http.StatusUnauthorized)
				if calls != tc.calls {
					t.Fatalf("Authenticate called %d times, want %d", calls, tc.calls)
				}
			}
		})
	}
}

func TestCreateGoalHTTP(t *testing.T) {
	owner := uuid.New()
	item := goal.Goal{ID: uuid.New(), UserID: owner, Name: "Trip", TargetMinor: 10000, SavedMinor: 0,
		CreatedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	const valid = `{"name":"Trip","targetMinor":10000}`
	const secret = "private-database-error"
	for _, tc := range []struct {
		name, body string
		serviceErr error
		status     int
		calls      int
	}{
		{"valid", valid, nil, http.StatusCreated, 1},
		{"empty name", `{"name":"  ","targetMinor":100}`, nil, http.StatusBadRequest, 0},
		{"zero target", `{"name":"Trip","targetMinor":0}`, nil, http.StatusBadRequest, 0},
		{"negative target", `{"name":"Trip","targetMinor":-1}`, nil, http.StatusBadRequest, 0},
		{"malformed JSON", `{`, nil, http.StatusBadRequest, 0},
		{"noninteger target", `{"name":"Trip","targetMinor":1.5}`, nil, http.StatusBadRequest, 0},
		{"unknown owner field", `{"name":"Trip","targetMinor":100,"userId":"` + uuid.NewString() + `"}`, nil, http.StatusBadRequest, 0},
		{"extra JSON", valid + ` {}`, nil, http.StatusBadRequest, 0},
		{"service invalid name", valid, goal.ErrInvalidName, http.StatusBadRequest, 1},
		{"service invalid target", valid, goal.ErrInvalidTarget, http.StatusBadRequest, 1},
		{"database failure", valid, errors.New(secret), http.StatusInternalServerError, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sessions := financialSessionStub{authenticate: func(_ context.Context, token string) (uuid.UUID, error) {
				if token != "token" {
					t.Errorf("unexpected token %q", token)
				}
				return owner, nil
			}}
			goals := goalServiceStub{create: func(_ context.Context, input goal.CreateInput) (goal.Goal, error) {
				calls++
				if input != (goal.CreateInput{UserID: owner, Name: "Trip", TargetMinor: 10000}) {
					t.Errorf("Create input = %+v", input)
				}
				return item, tc.serviceErr
			}}
			router := newRouter(api.NewServer(nil, api.WithFinancialServices(sessions, financialTransactionStub{}), api.WithGoalService(goals)))
			response := goalRequest(t, router, http.MethodPost, "/goals", tc.body, "Bearer token")
			if calls != tc.calls {
				t.Errorf("Create calls = %d, want %d", calls, tc.calls)
			}
			if tc.status == http.StatusCreated {
				assertGoalBody(t, response, tc.status, item)
			} else {
				assertGoalError(t, response, tc.status)
				if strings.Contains(response.Body.String(), secret) {
					t.Error("database detail leaked")
				}
			}
		})
	}
}

func TestListGoalsHTTP(t *testing.T) {
	owner := uuid.New()
	item := goal.Goal{ID: uuid.New(), UserID: owner, Name: "Emergency", TargetMinor: 25000, SavedMinor: 420,
		CreatedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)}
	inputs := []goal.ListInput{}
	goals := goalServiceStub{list: func(_ context.Context, input goal.ListInput) ([]goal.Goal, error) {
		inputs = append(inputs, input)
		if input.Offset > 0 {
			return nil, nil
		}
		return []goal.Goal{item}, nil
	}}
	router := newRouter(api.NewServer(nil, api.WithFinancialServices(financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return owner, nil }}, financialTransactionStub{}), api.WithGoalService(goals)))
	for _, tc := range []struct {
		path                string
		limit, offset, size int
	}{{"/goals", goal.DefaultListLimit, 0, 1}, {"/goals?limit=1&offset=1", 1, 1, 0}} {
		response := goalRequest(t, router, http.MethodGet, tc.path, "", "Bearer token")
		if response.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", tc.path, response.Code, response.Body.String())
		}
		var page struct {
			Data   []json.RawMessage `json:"data"`
			Limit  int               `json:"limit"`
			Offset int               `json:"offset"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Data == nil || len(page.Data) != tc.size || page.Limit != tc.limit || page.Offset != tc.offset {
			t.Fatalf("%s: unexpected page %+v", tc.path, page)
		}
		if tc.size == 1 {
			entry := httptest.NewRecorder()
			entry.Code = http.StatusOK
			entry.Body.Write(page.Data[0])
			assertGoalBody(t, entry, http.StatusOK, item)
		}
		if got := inputs[len(inputs)-1]; got != (goal.ListInput{UserID: owner, Limit: tc.limit, Offset: tc.offset}) {
			t.Errorf("List input = %+v", got)
		}
	}
	for _, path := range []string{"/goals?limit=0", "/goals?limit=101", "/goals?offset=-1", "/goals?limit=abc", "/goals?limit=", "/goals?limit=1&limit=2", "/goals?offset="} {
		response := goalRequest(t, router, http.MethodGet, path, "", "Bearer token")
		assertGoalError(t, response, http.StatusBadRequest)
		if len(inputs) != 2 {
			t.Fatalf("List called for invalid pagination %s", path)
		}
	}
	dbFailure := goalServiceStub{list: func(context.Context, goal.ListInput) ([]goal.Goal, error) {
		return nil, errors.New("private-database-error")
	}}
	router = newRouter(api.NewServer(nil, api.WithFinancialServices(financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return owner, nil }}, financialTransactionStub{}), api.WithGoalService(dbFailure)))
	response := goalRequest(t, router, http.MethodGet, "/goals", "", "Bearer token")
	assertGoalError(t, response, http.StatusInternalServerError)
	if strings.Contains(response.Body.String(), "private-database-error") {
		t.Error("database detail leaked")
	}
}

func TestAdjustGoalHTTP(t *testing.T) {
	owner, id := uuid.New(), uuid.New()
	item := goal.Goal{ID: id, UserID: owner, Name: "Trip", TargetMinor: 10000, SavedMinor: 250,
		CreatedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)}
	const deposit = `{"direction":"deposit","amountMinor":250}`
	const withdraw = `{"direction":"withdraw","amountMinor":250}`
	const secret = "private-database-error"
	for _, tc := range []struct {
		name, path, body string
		serviceErr       error
		status           int
		methodCalls      int
	}{
		{"deposit", "/goals/" + id.String() + "/allocations", deposit, nil, http.StatusOK, 1},
		{"withdraw", "/goals/" + id.String() + "/allocations", withdraw, nil, http.StatusOK, 1},
		{"missing goal", "/goals/" + id.String() + "/allocations", deposit, goal.ErrNotFound, http.StatusNotFound, 1},
		{"other owner", "/goals/" + id.String() + "/allocations", withdraw, goal.ErrNotFound, http.StatusNotFound, 1},
		{"insufficient saved funds", "/goals/" + id.String() + "/allocations", withdraw, goal.ErrInsufficientFunds, http.StatusBadRequest, 1},
		{"invalid service amount", "/goals/" + id.String() + "/allocations", deposit, goal.ErrInvalidAmount, http.StatusBadRequest, 1},
		{"database failure", "/goals/" + id.String() + "/allocations", deposit, errors.New(secret), http.StatusInternalServerError, 1},
		{"malformed JSON", "/goals/" + id.String() + "/allocations", `{`, nil, http.StatusBadRequest, 0},
		{"missing direction", "/goals/" + id.String() + "/allocations", `{"amountMinor":250}`, nil, http.StatusBadRequest, 0},
		{"invalid direction", "/goals/" + id.String() + "/allocations", `{"direction":"transfer","amountMinor":250}`, nil, http.StatusBadRequest, 0},
		{"zero amount", "/goals/" + id.String() + "/allocations", `{"direction":"deposit","amountMinor":0}`, nil, http.StatusBadRequest, 0},
		{"negative amount", "/goals/" + id.String() + "/allocations", `{"direction":"withdraw","amountMinor":-1}`, nil, http.StatusBadRequest, 0},
		{"noninteger amount", "/goals/" + id.String() + "/allocations", `{"direction":"deposit","amountMinor":1.5}`, nil, http.StatusBadRequest, 0},
		{"unknown owner field", "/goals/" + id.String() + "/allocations", `{"direction":"deposit","amountMinor":250,"userId":"` + uuid.NewString() + `"}`, nil, http.StatusBadRequest, 0},
		{"nil ID", "/goals/" + uuid.Nil.String() + "/allocations", deposit, nil, http.StatusNotFound, 0},
		{"invalid UUID", "/goals/not-a-uuid/allocations", deposit, nil, http.StatusBadRequest, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			wantMethod := "deposit"
			if strings.Contains(tc.body, `"direction":"withdraw"`) {
				wantMethod = "withdraw"
			}
			adjust := func(method string, userID, goalID uuid.UUID, amount int64) (goal.Goal, error) {
				calls++
				if method != wantMethod || userID != owner || goalID != id || amount != 250 {
					t.Errorf("%s arguments = %s, %s, %d; want %s, %s, %s, 250", method, userID, goalID, amount, wantMethod, owner, id)
				}
				return item, tc.serviceErr
			}
			goals := goalServiceStub{
				deposit: func(_ context.Context, userID, goalID uuid.UUID, amount int64) (goal.Goal, error) {
					return adjust("deposit", userID, goalID, amount)
				},
				withdraw: func(_ context.Context, userID, goalID uuid.UUID, amount int64) (goal.Goal, error) {
					return adjust("withdraw", userID, goalID, amount)
				},
			}
			router := newRouter(api.NewServer(nil, api.WithFinancialServices(financialSessionStub{authenticate: func(context.Context, string) (uuid.UUID, error) { return owner, nil }}, financialTransactionStub{}), api.WithGoalService(goals)))
			response := goalRequest(t, router, http.MethodPost, tc.path, tc.body, "Bearer token")
			if calls != tc.methodCalls {
				t.Errorf("adjust calls = %d, want %d", calls, tc.methodCalls)
			}
			if tc.status == http.StatusOK {
				assertGoalBody(t, response, tc.status, item)
			} else {
				assertGoalError(t, response, tc.status)
				if strings.Contains(response.Body.String(), secret) {
					t.Error("database detail leaked")
				}
			}
		})
	}
}
