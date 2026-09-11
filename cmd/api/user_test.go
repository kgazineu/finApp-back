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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/user"
)

type userCreatorStub struct {
	called   bool
	received user.CreateInput
	result   user.User
	err      error
}

func (s *userCreatorStub) List(context.Context, user.ListInput) ([]user.User, error) {
	panic("List não deveria ser chamado nos testes de criação")
}

func (s *userCreatorStub) Create(
	ctx context.Context,
	input user.CreateInput,
) (user.User, error) {
	s.called = true
	s.received = input
	return s.result, s.err
}

func TestCreateUserMapsServiceErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"nome inválido", user.ErrInvalidName, 400, "nome não pode ficar vazio"},
		{"e-mail inválido", user.ErrInvalidEmail, 400, "e-mail inválido"},
		{"senha inválida", user.ErrInvalidPassword, 400, "senha deve conter entre 15 e 128 caracteres"},
		{"e-mail duplicado", fmt.Errorf("cadastro: %w", user.ErrEmailAlreadyExists), 409, "e-mail já cadastrado"},
		{"falha interna", errors.New("detalhe interno que não pode vazar"), 500, "Não foi possível cadastrar o usuário"},
		{"hash inválido é falha interna", user.ErrInvalidPasswordHash, 500, "Não foi possível cadastrar o usuário"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &userCreatorStub{err: tt.err}
			router := newRouter(api.NewServer(service))
			request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Kaian","email":"kaian@example.com","password":"uma-senha-de-teste-123!"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status esperado %d, recebido %d", tt.status, response.Code)
			}
			var body api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Message != tt.message {
				t.Errorf("mensagem inesperada: %q", body.Message)
			}
		})
	}
}

func TestCreateUserRejectsInvalidBodyBeforeCallingService(t *testing.T) {
	for _, tt := range []struct {
		name, body, contentType string
		status                  int
	}{
		{"JSON incompleto", `{`, "application/json", 400},
		{"corpo vazio", ``, "application/json", 400},
		{"null", `null`, "application/json", 400},
		{"senha ausente", `{"name":"Kaian","email":"kaian@example.com"}`, "application/json", 400},
		{"nome ausente", `{"email":"kaian@example.com","password":"senha-de-teste-123"}`, "application/json", 400},
		{"e-mail ausente", `{"name":"Kaian","password":"senha-de-teste-123"}`, "application/json", 400},
		{"tipo incorreto", `{"name":123}`, "application/json", 400},
		{"dois objetos", `{"name":"Kaian","email":"kaian@example.com","password":"senha-de-teste-123"} {}`, "application/json", 400},
		{"formato incorreto", `{}`, "text/plain", 415},
		{"corpo excessivo", `{"name":"` + strings.Repeat("a", 65536) + `"}`, "application/json", 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &userCreatorStub{}
			request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			response := httptest.NewRecorder()
			newRouter(api.NewServer(service)).ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Errorf("status esperado %d, recebido %d", tt.status, response.Code)
			}
			if service.called {
				t.Error("serviço chamado para corpo inválido")
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	created := user.User{
		ID:           uuid.New(),
		Name:         "Kaian",
		Email:        "kaian@example.com",
		PasswordHash: "hash-que-nao-deve-aparecer-na-resposta",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	service := &userCreatorStub{
		result: created,
	}

	router := newRouter(api.NewServer(service))

	body := strings.NewReader(`{
		"name": "Kaian",
		"email": "kaian@example.com",
		"password": "uma-senha-de-teste-123!"
	}`)

	request := httptest.NewRequest(http.MethodPost, "/users", body)
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf(
			"status esperado %d, recebido %d",
			http.StatusCreated,
			response.Code,
		)
	}

	if !service.called {
		t.Fatal("o serviço de cadastro deveria ter sido chamado")
	}

	expectedInput := user.CreateInput{
		Name:     "Kaian",
		Email:    "kaian@example.com",
		Password: "uma-senha-de-teste-123!",
	}

	if service.received != expectedInput {
		t.Error("o serviço recebeu dados diferentes da requisição")
	}

	var got map[string]any

	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("erro ao interpretar a resposta JSON: %v", err)
	}

	expected := map[string]string{
		"id":        created.ID.String(),
		"name":      created.Name,
		"email":     created.Email,
		"createdAt": now.Format(time.RFC3339),
		"updatedAt": now.Format(time.RFC3339),
	}

	if len(got) != len(expected) {
		t.Errorf(
			"esperados apenas os %d campos públicos, recebidos %d campos",
			len(expected),
			len(got),
		)
	}

	for field, value := range expected {
		if got[field] != value {
			t.Errorf("campo %q ausente ou com valor diferente do esperado", field)
		}
	}
}
