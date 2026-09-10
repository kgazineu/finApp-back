package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestUserRepositoryCreate(t *testing.T) {
	tx := newTestDB(t)
	repo := postgres.NewUserRepository(tx)

	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	input := user.User{
		ID:           uuid.New(),
		Name:         "Kaian",
		Email:        "kaian@example.com",
		PasswordHash: "hash-ficticio-para-teste",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	created, err := repo.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("erro ao persistir usuário: %v", err)
	}

	if created.ID != input.ID {
		t.Error("o repositório deve preservar o ID recebido")
	}

	var stored user.User

	err = tx.Raw(`
		SELECT id, name, email, password_hash, created_at, updated_at
		FROM users
		WHERE id = ?
	`, input.ID).Row().Scan(
		&stored.ID,
		&stored.Name,
		&stored.Email,
		&stored.PasswordHash,
		&stored.CreatedAt,
		&stored.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("erro ao consultar usuário persistido: %v", err)
	}

	if stored.ID != input.ID ||
		stored.Name != input.Name ||
		stored.Email != input.Email ||
		stored.PasswordHash != input.PasswordHash {
		t.Error("os dados persistidos diferem dos dados enviados")
	}

	if !stored.CreatedAt.Equal(input.CreatedAt) ||
		!stored.UpdatedAt.Equal(input.UpdatedAt) {
		t.Error("as datas persistidas diferem das datas enviadas")
	}
}

func TestUserRepositoryCreateRejectsDuplicateEmail(t *testing.T) {
	tx := newTestDB(t)
	repo := postgres.NewUserRepository(tx)
	ctx := context.Background()

	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	first := user.User{
		ID:           uuid.New(),
		Name:         "Kaian",
		Email:        "kaian@example.com",
		PasswordHash: "hash-ficticio-para-teste",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if _, err := repo.Create(ctx, first); err != nil {
		t.Fatalf("erro ao persistir primeiro usuário: %v", err)
	}

	second := first
	second.ID = uuid.New()
	second.Name = "Outro usuário"

	_, err := repo.Create(ctx, second)

	if !errors.Is(err, user.ErrEmailAlreadyExists) {
		t.Errorf(
			"esperado ErrEmailAlreadyExists, recebido: %v",
			err,
		)
	}
}

func TestDuplicateIDIsNotReportedAsDuplicateEmail(t *testing.T) {
	tx := newTestDB(t)
	repo := postgres.NewUserRepository(tx)
	u, err := user.New("Kaian", "first@example.com", "hash-de-teste")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	u.Email = "second@example.com"
	_, err = repo.Create(context.Background(), u)
	if err == nil {
		t.Fatal("esperado conflito da chave primária")
	}
	if errors.Is(err, user.ErrEmailAlreadyExists) {
		t.Fatal("conflito de ID traduzido como e-mail duplicado")
	}
}
