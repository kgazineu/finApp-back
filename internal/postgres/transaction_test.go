package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

func transactionOwner(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	owner, err := user.New("Titular", uuid.NewString()+"@example.com", "hash-de-teste")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewUserRepository(db).Create(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	return owner.ID
}

func TestTransactionRepositoryCreate(t *testing.T) {
	db := newTestDB(t)
	ownerID := transactionOwner(t, db)
	repo := postgres.NewTransactionRepository(db)
	createdAt := time.Date(2026, time.September, 29, 12, 30, 0, 123456000, time.UTC)
	input := transaction.Transaction{
		ID: uuid.New(), UserID: ownerID, AmountMinor: 9223372036854775807,
		NecessityLevel: 5, CreatedAt: createdAt,
	}
	created, err := repo.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if created != input {
		t.Errorf("transação alterada na persistência: got=%+v want=%+v", created, input)
	}
	var owner uuid.UUID
	var amount int64
	var level int
	var storedAt time.Time
	if err := db.Raw("SELECT user_id, amount_minor, necessity_level, created_at FROM transactions WHERE id = ?", input.ID).
		Row().Scan(&owner, &amount, &level, &storedAt); err != nil {
		t.Fatal(err)
	}
	if owner != ownerID || amount != input.AmountMinor || level != 5 || !storedAt.Equal(createdAt) {
		t.Errorf("valores persistidos incorretos: owner=%s amount=%d level=%d created_at=%s", owner, amount, level, storedAt)
	}
}

func TestTransactionRepositoryRejectsUnknownOwner(t *testing.T) {
	db := newTestDB(t)
	input, err := transaction.New(uuid.New(), 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = postgres.NewTransactionRepository(db).Create(context.Background(), input)
	if err == nil {
		t.Fatal("esperado erro de chave estrangeira para titular inexistente")
	}
}

func TestTransactionsDatabaseConstraints(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int64
		level  int
	}{
		{"valor zero", 0, 1},
		{"valor negativo", -1, 1},
		{"nível zero", 100, 0},
		{"nível seis", 100, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			ownerID := transactionOwner(t, db)
			err := db.Exec(`INSERT INTO transactions (id, user_id, amount_minor, necessity_level, created_at)
				VALUES (?, ?, ?, ?, ?)`, uuid.New(), ownerID, tc.amount, tc.level, time.Now().UTC()).Error
			if err == nil {
				t.Fatal("banco aceitou transação inválida")
			}
		})
	}
}

func TestTransactionRepositoryHonorsCanceledContext(t *testing.T) {
	db, _ := testutil.Database(t)
	ownerID := transactionOwner(t, db)
	input, err := transaction.New(ownerID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = postgres.NewTransactionRepository(db).Create(ctx, input)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("contexto cancelado ignorado: %v", err)
	}
}
