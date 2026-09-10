package user_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestNewCreatesUser(t *testing.T) {
	name := "Kaian"
	email := "kaian@example.com"
	passwordHash := "hash-ficticio-para-teste"

	got, err := user.New(name, email, passwordHash)

	if err != nil {
		t.Fatalf("erro inesperado ao criar usuário: %v", err)
	}

	if got.ID == uuid.Nil {
		t.Error("usuário criado sem UUID")
	}

	if got.Name != name {
		t.Errorf("nome esperado %q, recebido %q", name, got.Name)
	}

	if got.Email != email {
		t.Errorf("e-mail esperado %q, recebido %q", email, got.Email)
	}

	if got.PasswordHash != passwordHash {
		t.Error("hash da senha diferente do informado")
	}

	if got.CreatedAt.IsZero() {
		t.Error("data de criação não preenchida")
	}

	if got.UpdatedAt.IsZero() {
		t.Error("data de atualização não preenchida")
	}

	if !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Error("as datas de criação e atualização devem ser iguais no cadastro")
	}
}

func TestNewRejectsBlankName(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "nome vazio",
			input: "",
		},
		{
			name:  "apenas espaços",
			input: "   ",
		},
		{
			name:  "tabulação e quebra de linha",
			input: "\t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := user.New(
				tt.input,
				"kaian@example.com",
				"hash-ficticio-para-teste",
			)

			if err == nil {
				t.Errorf(
					"esperado erro para nome %q, recebido nil",
					tt.input,
				)
			}
		})
	}
}

func TestNewRejectsInvalidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{
			name:  "e-mail vazio",
			email: "",
		},
		{
			name:  "apenas espaços",
			email: "   ",
		},
		{
			name:  "sem arroba",
			email: "kaian.example.com",
		},
		{
			name:  "sem parte local",
			email: "@example.com",
		},
		{
			name:  "sem domínio",
			email: "kaian@",
		},
		{
			name:  "com nome de exibição",
			email: "Kaian <kaian@example.com>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := user.New(
				"Kaian",
				tt.email,
				"hash-ficticio-para-teste",
			)

			if err == nil {
				t.Errorf(
					"esperado erro para e-mail %q, recebido nil",
					tt.email,
				)
			}
		})
	}
}

func TestNewRejectsBlankPasswordHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{
			name: "hash vazio",
			hash: "",
		},
		{
			name: "apenas espaços",
			hash: "   ",
		},
		{
			name: "tabulação e quebra de linha",
			hash: "\t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := user.New(
				"Kaian",
				"kaian@example.com",
				tt.hash,
			)

			if err == nil {
				t.Error("esperado erro para hash em branco, recebido nil")
			}
		})
	}
}
