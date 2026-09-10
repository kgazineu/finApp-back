package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

type hasherStub struct {
	called   bool
	received string
	result   string
	err      error
}

func TestServiceCreatePasswordLength(t *testing.T) {
	for _, tt := range []struct {
		name, password string
		valid          bool
	}{
		{"14 caracteres", strings.Repeat("a", 14), false},
		{"15 caracteres", strings.Repeat("a", 15), true},
		{"128 caracteres", strings.Repeat("a", 128), true},
		{"129 caracteres", strings.Repeat("a", 129), false},
		{"Unicode curto em caracteres", strings.Repeat("é", 8), false},
		{"Unicode aceito", strings.Repeat("é", 15), true},
		{"espaços preservados", "  frase de senha  ", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hasher := &hasherStub{result: "hash-de-teste"}
			repo := &repositoryStub{}
			_, err := user.NewService(repo, hasher).Create(context.Background(), user.CreateInput{Name: "Kaian", Email: "kaian@example.com", Password: tt.password})
			if tt.valid {
				if err != nil {
					t.Fatal(err)
				}
				if hasher.received != tt.password {
					t.Error("senha alterada antes do hashing")
				}
			} else {
				if !errors.Is(err, user.ErrInvalidPassword) {
					t.Errorf("esperado ErrInvalidPassword, recebido %v", err)
				}
				if hasher.called || repo.called {
					t.Error("dependências chamadas para senha inválida")
				}
			}
		})
	}
}

func TestServiceCreateValidatesProfileBeforeHashing(t *testing.T) {
	for _, input := range []user.CreateInput{
		{Name: "   ", Email: "kaian@example.com", Password: "senha-de-teste-123"},
		{Name: "Kaian", Email: "inválido", Password: "senha-de-teste-123"},
	} {
		hasher := &hasherStub{result: "hash-de-teste"}
		repo := &repositoryStub{}
		_, err := user.NewService(repo, hasher).Create(context.Background(), input)
		if err == nil {
			t.Error("esperado erro de validação")
		}
		if hasher.called || repo.called {
			t.Error("dependências chamadas para perfil inválido")
		}
	}
}

func (h *hasherStub) Hash(plain string) (string, error) {
	h.called = true
	h.received = plain
	return h.result, h.err
}

type repositoryStub struct {
	called   bool
	received user.User
	err      error
}

func (r *repositoryStub) Create(
	ctx context.Context,
	u user.User,
) (user.User, error) {
	r.called = true
	r.received = u

	if r.err != nil {
		return user.User{}, r.err
	}

	return u, nil
}

func TestServiceCreateHashesPasswordAndPersistsUser(t *testing.T) {
	hasher := &hasherStub{
		result: "hash-gerado-pelo-componente",
	}
	repo := &repositoryStub{}
	service := user.NewService(repo, hasher)

	input := user.CreateInput{
		Name:     "Kaian",
		Email:    "kaian@example.com",
		Password: "uma-senha-de-teste-123!",
	}

	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("erro inesperado no cadastro: %v", err)
	}

	if hasher.received != input.Password {
		t.Error("o componente de hashing deve receber a senha informada")
	}

	if !repo.called {
		t.Fatal("o repositório deveria ter sido chamado")
	}

	if repo.received.PasswordHash != hasher.result {
		t.Error("o repositório deve receber o hash gerado")
	}

	if repo.received.Name != input.Name ||
		repo.received.Email != input.Email {
		t.Error("o repositório deve receber o nome e o e-mail informados")
	}

	if repo.received.ID == uuid.Nil {
		t.Error("o usuário deve possuir um UUID antes da persistência")
	}

	if repo.received.CreatedAt.IsZero() ||
		repo.received.UpdatedAt.IsZero() {
		t.Error("o usuário deve possuir as datas antes da persistência")
	}

	if created != repo.received {
		t.Error("o serviço deve retornar o usuário devolvido pelo repositório")
	}
}

func TestServiceCreateRejectsEmptyPassword(t *testing.T) {
	hasher := &hasherStub{
		result: "hash-gerado-pelo-componente",
	}
	repo := &repositoryStub{}
	service := user.NewService(repo, hasher)

	input := user.CreateInput{
		Name:     "Kaian",
		Email:    "kaian@example.com",
		Password: "",
	}

	_, err := service.Create(context.Background(), input)

	if !errors.Is(err, user.ErrInvalidPassword) {
		t.Errorf("esperado ErrInvalidPassword, recebido: %v", err)
	}

	if hasher.called {
		t.Error("o hasher não deveria ser chamado para senha vazia")
	}

	if repo.called {
		t.Error("o repositório não deveria ser chamado para senha vazia")
	}
}

func TestServiceCreateStopsWhenHashingFails(t *testing.T) {
	hashErr := errors.New("falha simulada no hashing")
	hasher := &hasherStub{
		err: hashErr,
	}
	repo := &repositoryStub{}
	service := user.NewService(repo, hasher)

	input := user.CreateInput{
		Name:     "Kaian",
		Email:    "kaian@example.com",
		Password: "uma-senha-de-teste-123!",
	}

	_, err := service.Create(context.Background(), input)

	if !errors.Is(err, hashErr) {
		t.Errorf("esperado preservar o erro de hashing, recebido: %v", err)
	}

	if repo.called {
		t.Error("o repositório não deveria ser chamado quando o hashing falha")
	}
}

func TestServiceCreatePreservesRepositoryErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "e-mail já cadastrado",
			err:  user.ErrEmailAlreadyExists,
		},
		{
			name: "falha inesperada na persistência",
			err:  errors.New("falha simulada no banco"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasher := &hasherStub{
				result: "hash-gerado-pelo-componente",
			}
			repo := &repositoryStub{
				err: tt.err,
			}
			service := user.NewService(repo, hasher)

			input := user.CreateInput{
				Name:     "Kaian",
				Email:    "kaian@example.com",
				Password: "uma-senha-de-teste-123!",
			}

			_, err := service.Create(context.Background(), input)

			if !errors.Is(err, tt.err) {
				t.Errorf(
					"esperado preservar o erro %v, recebido: %v",
					tt.err,
					err,
				)
			}

			if !repo.called {
				t.Error("o repositório deveria ter sido chamado")
			}
		})
	}
}
