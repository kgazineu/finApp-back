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
	"github.com/kgazineu/finApp-back/internal/user"
)

type userListerStub struct {
	userCreatorStub
	list func(context.Context, user.ListInput) ([]user.User, error)
}

func (s *userListerStub) List(ctx context.Context, input user.ListInput) ([]user.User, error) {
	return s.list(ctx, input)
}

func TestListUsersReturnsPublicFieldsAndPagination(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	u := user.User{ID: uuid.New(), Name: "Kaian", Email: "kaian@example.com", PasswordHash: "hash-secreto", CreatedAt: now, UpdatedAt: now}
	for _, tt := range []struct {
		path  string
		input user.ListInput
	}{
		{"/users", user.ListInput{Limit: 20, Offset: 0}},
		{"/users?limit=1&offset=2", user.ListInput{Limit: 1, Offset: 2}},
		{"/users?limit=100", user.ListInput{Limit: 100, Offset: 0}},
	} {
		t.Run(tt.path, func(t *testing.T) {
			called := false
			service := &userListerStub{list: func(ctx context.Context, input user.ListInput) ([]user.User, error) {
				called = true
				if input != tt.input {
					t.Errorf("esperado %+v, recebido %+v", tt.input, input)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("a consulta deve ter prazo de execução")
				}
				return []user.User{u}, nil
			}}
			response := httptest.NewRecorder()
			newRouter(api.NewServer(service)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != 200 || !called {
				t.Fatalf("esperado 200 e serviço chamado, recebido %d: %s", response.Code, response.Body)
			}
			var body struct {
				Data   []map[string]any `json:"data"`
				Limit  int              `json:"limit"`
				Offset int              `json:"offset"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Limit != tt.input.Limit || body.Offset != tt.input.Offset || len(body.Data) != 1 {
				t.Fatalf("página inesperada: %+v", body)
			}
			want := map[string]string{"id": u.ID.String(), "name": u.Name, "email": u.Email, "createdAt": now.Format(time.RFC3339), "updatedAt": now.Format(time.RFC3339)}
			if len(body.Data[0]) != len(want) {
				t.Fatal("a resposta deve conter apenas os cinco campos públicos")
			}
			for key, value := range want {
				if body.Data[0][key] != value {
					t.Errorf("campo %s: esperado %q, recebido %v", key, value, body.Data[0][key])
				}
			}
			if strings.Contains(response.Body.String(), u.PasswordHash) {
				t.Fatal("hash exposto na listagem")
			}
		})
	}
}

func TestListUsersReturnsEmptyArray(t *testing.T) {
	service := &userListerStub{list: func(context.Context, user.ListInput) ([]user.User, error) { return nil, nil }}
	response := httptest.NewRecorder()
	newRouter(api.NewServer(service)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users", nil))
	if response.Code != 200 {
		t.Fatalf("esperado 200, recebido %d", response.Code)
	}
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body.Data) != "[]" {
		t.Fatalf("esperado [], recebido %s", body.Data)
	}
}

func TestListUsersRejectsInvalidPagination(t *testing.T) {
	for _, query := range []string{
		"limit=0", "limit=-1", "limit=101", "offset=-1", "limit=abc", "offset=1.5",
		"limit=999999999999999999999", "offset=999999999999999999999", "limit=", "offset=",
		"limit=1&limit=2", "offset=0&offset=1",
	} {
		t.Run(query, func(t *testing.T) {
			service := &userListerStub{list: func(context.Context, user.ListInput) ([]user.User, error) {
				t.Fatal("o serviço não deve receber paginação inválida")
				return nil, nil
			}}
			response := httptest.NewRecorder()
			newRouter(api.NewServer(service)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users?"+query, nil))
			if response.Code != 400 {
				t.Fatalf("esperado 400, recebido %d: %s", response.Code, response.Body)
			}
			var body api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Message == "" {
				t.Fatal("erro deve seguir o contrato {message: ...}")
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
		{fmt.Errorf("listar: %w", user.ErrInvalidPagination), 400, user.ErrInvalidPagination.Error()},
		{errors.New("credencial-interna"), 500, "Não foi possível listar os usuários"},
	} {
		service := &userListerStub{list: func(context.Context, user.ListInput) ([]user.User, error) { return nil, tt.err }}
		response := httptest.NewRecorder()
		newRouter(api.NewServer(service)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users", nil))
		if response.Code != tt.status {
			t.Fatalf("esperado %d, recebido %d", tt.status, response.Code)
		}
		var body api.ErrorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Message != tt.message {
			t.Errorf("mensagem inesperada: %q", body.Message)
		}
	}
}

func TestListUsersWithoutServiceReturnsInternalError(t *testing.T) {
	response := httptest.NewRecorder()
	newRouter(api.NewServer(nil)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users", nil))
	if response.Code != 500 {
		t.Fatalf("esperado 500, recebido %d", response.Code)
	}
}
