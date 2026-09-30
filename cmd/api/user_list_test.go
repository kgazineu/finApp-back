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
	"github.com/kgazineu/finApp-back/internal/user"
)

type userListerStub struct {
	userCreatorStub
	get    func(context.Context, uuid.UUID) (user.User, error)
	update func(context.Context, user.UpdateInput) (user.User, error)
}

func (s *userListerStub) List(context.Context, user.ListInput) ([]user.User, error) {
	panic("global List must never be called from GET /users")
}

func (s *userListerStub) Get(ctx context.Context, id uuid.UUID) (user.User, error) {
	if s.get == nil {
		panic("unexpected Get call")
	}
	return s.get(ctx, id)
}

func (s *userListerStub) Update(ctx context.Context, input user.UpdateInput) (user.User, error) {
	if s.update == nil {
		panic("unexpected Update call")
	}
	return s.update(ctx, input)
}

type userSessionStub struct {
	id  uuid.UUID
	err error
}

func (s userSessionStub) Login(context.Context, string, string) (string, time.Time, error) {
	panic("unexpected Login call")
}

func (s userSessionStub) Authenticate(ctx context.Context, token string) (uuid.UUID, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("session authentication must have a deadline")
	}
	if token != "valid-token" {
		return uuid.Nil, auth.ErrInvalidSession
	}
	return s.id, s.err
}

func userRouter(service api.UserService, session userSessionStub) http.Handler {
	return newRouter(api.NewServer(service, api.WithFinancialServices(session, nil)))
}

func userRequest(method, path, body, authorization string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return request
}

func TestListUsersReturnsOnlyAuthenticatedProfileAndPagination(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	u := user.User{ID: uuid.New(), Name: "Kaian", Email: "kaian@example.com", PasswordHash: "hash-secreto", CreatedAt: now, UpdatedAt: now}
	for _, tt := range []struct {
		path   string
		limit  int
		offset int
	}{
		{"/users", 20, 0},
		{"/users?limit=1&offset=2", 1, 2},
		{"/users?limit=100", 100, 0},
	} {
		t.Run(tt.path, func(t *testing.T) {
			calls := 0
			service := &userListerStub{get: func(ctx context.Context, id uuid.UUID) (user.User, error) {
				calls++
				if id != u.ID {
					t.Errorf("looked up a different user: %s", id)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("lookup must have a deadline")
				}
				return u, nil
			}}
			response := httptest.NewRecorder()
			userRouter(service, userSessionStub{id: u.ID}).ServeHTTP(response, userRequest(http.MethodGet, tt.path, "", "Bearer valid-token"))
			wantCalls := 1
			if tt.offset > 0 {
				wantCalls = 0
			}
			if response.Code != 200 || calls != wantCalls {
				t.Fatalf("expected 200 and %d lookups, got %d and %d: %s", wantCalls, response.Code, calls, response.Body)
			}
			var body struct {
				Data   []map[string]any `json:"data"`
				Limit  int              `json:"limit"`
				Offset int              `json:"offset"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Limit != tt.limit || body.Offset != tt.offset || body.Data == nil || len(body.Data) != wantCalls {
				t.Fatalf("unexpected page: %+v", body)
			}
			if wantCalls == 0 {
				return
			}
			want := map[string]string{"id": u.ID.String(), "name": u.Name, "email": u.Email, "createdAt": now.Format(time.RFC3339), "updatedAt": now.Format(time.RFC3339)}
			if len(body.Data[0]) != len(want) {
				t.Fatal("response must contain only five public fields")
			}
			for key, value := range want {
				if body.Data[0][key] != value {
					t.Errorf("field %s: expected %q, got %v", key, value, body.Data[0][key])
				}
			}
			if strings.Contains(response.Body.String(), u.PasswordHash) {
				t.Fatal("password hash leaked")
			}
		})
	}
}

func TestListUsersRejectsInvalidPagination(t *testing.T) {
	for _, query := range []string{
		"limit=0", "limit=-1", "limit=101", "offset=-1", "limit=abc", "offset=1.5",
		"limit=999999999999999999999", "offset=999999999999999999999", "limit=", "offset=",
		"limit=1&limit=2", "offset=0&offset=1",
	} {
		t.Run(query, func(t *testing.T) {
			service := &userListerStub{}
			response := httptest.NewRecorder()
			userRouter(service, userSessionStub{id: uuid.New()}).ServeHTTP(response, userRequest(http.MethodGet, "/users?"+query, "", "Bearer valid-token"))
			if response.Code != 400 {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body)
			}
			var body api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message == "" {
				t.Fatalf("expected JSON error: %s (%v)", response.Body, err)
			}
		})
	}
}

func TestListUsersMapsServiceErrors(t *testing.T) {
	for _, tt := range []struct {
		err     error
		status  int
		message string
	}{
		{fmt.Errorf("lookup: %w", user.ErrUserNotFound), 404, user.ErrUserNotFound.Error()},
		{errors.New("internal-secret"), 500, "Não foi possível listar os usuários"},
	} {
		service := &userListerStub{get: func(context.Context, uuid.UUID) (user.User, error) { return user.User{}, tt.err }}
		response := httptest.NewRecorder()
		userRouter(service, userSessionStub{id: uuid.New()}).ServeHTTP(response, userRequest(http.MethodGet, "/users", "", "Bearer valid-token"))
		if response.Code != tt.status {
			t.Fatalf("expected %d, got %d", tt.status, response.Code)
		}
		var body api.ErrorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Message != tt.message {
			t.Errorf("unexpected error: %+v (%v)", body, err)
		}
	}
}

func TestListUsersRequiresSession(t *testing.T) {
	service := &userListerStub{}
	for _, tt := range []struct {
		authorization string
		sessionErr    error
		status        int
	}{
		{"", nil, 401},
		{"Basic valid-token", nil, 401},
		{"Bearer invalid", nil, 401},
		{"Bearer valid-token", auth.ErrInvalidSession, 401},
		{"Bearer valid-token", errors.New("database failure"), 500},
	} {
		response := httptest.NewRecorder()
		userRouter(service, userSessionStub{id: uuid.New(), err: tt.sessionErr}).ServeHTTP(response, userRequest(http.MethodGet, "/users", "", tt.authorization))
		if response.Code != tt.status {
			t.Errorf("authorization %q: expected %d, got %d: %s", tt.authorization, tt.status, response.Code, response.Body)
		}
		if tt.status == 401 && response.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Error("missing Bearer challenge")
		}
	}
}

func TestListUsersCanceledRequestDoesNotReturnAnEmptyPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := userRequest(http.MethodGet, "/users?offset=1", "", "Bearer valid-token").WithContext(ctx)
	response := httptest.NewRecorder()
	userRouter(&userListerStub{}, userSessionStub{id: uuid.New()}).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for canceled request, got %d: %s", response.Code, response.Body)
	}
}

func TestListUsersWithoutServiceReturnsInternalError(t *testing.T) {
	response := httptest.NewRecorder()
	userRouter(nil, userSessionStub{id: uuid.New()}).ServeHTTP(response, userRequest(http.MethodGet, "/users", "", "Bearer valid-token"))
	if response.Code != 500 {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}

func TestUpdateUserRequiresOwnerBeforeBodyDecode(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	service := &userListerStub{}
	for _, tt := range []struct {
		name, authorization, path, body string
		status                          int
	}{
		{"anonymous", "", "/users/" + owner.String(), "{", 401},
		{"other owner", "Bearer valid-token", "/users/" + other.String(), "{", 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := userRequest(http.MethodPatch, tt.path, tt.body, tt.authorization)
			request.Header.Set("Content-Type", "text/plain")
			response := httptest.NewRecorder()
			userRouter(service, userSessionStub{id: owner}).ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("expected %d before body decode, got %d: %s", tt.status, response.Code, response.Body)
			}
		})
	}
}

func TestUpdateUserOwnProfile(t *testing.T) {
	owner := uuid.New()
	service := &userListerStub{update: func(ctx context.Context, input user.UpdateInput) (user.User, error) {
		if _, ok := ctx.Deadline(); !ok || input.ID != owner || input.Name == nil || *input.Name != "New name" {
			t.Errorf("unexpected update input: %+v", input)
		}
		return user.User{ID: owner, Name: "New name", Email: "owner@example.com", PasswordHash: "secret"}, nil
	}}
	request := userRequest(http.MethodPatch, "/users/"+owner.String(), `{"name":"New name"}`, "Bearer valid-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	userRouter(service, userSessionStub{id: owner}).ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "New name") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unexpected self-update response %d: %s", response.Code, response.Body)
	}
}
