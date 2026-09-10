package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/user"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUserRepositoryCreate(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("defina TEST_DATABASE_URL para executar a integração")
	}

	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("erro ao conectar ao banco de testes: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("erro ao acessar o pool de conexões: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("erro ao fechar conexões: %v", err)
		}
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("erro ao iniciar transação: %v", tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("erro ao desfazer transação: %v", err)
		}
	})

	migration, err := os.ReadFile(
		"../../migrations/000001_create_users.up.sql",
	)
	if err != nil {
		t.Fatalf("erro ao ler migration: %v", err)
	}

	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatalf("erro ao aplicar migration: %v", err)
	}

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
