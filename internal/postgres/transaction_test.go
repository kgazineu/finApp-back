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
	if created.ID != input.ID || created.UserID != input.UserID || created.AmountMinor != input.AmountMinor ||
		created.NecessityLevel != input.NecessityLevel || !created.CreatedAt.Equal(createdAt) ||
		created.Kind != "expense" || created.Description != "Lançamento" || created.Category != "Outros" ||
		created.Installments != 1 || !created.OccurredAt.Equal(createdAt) {
		t.Errorf("defaults legados incorretos: %+v", created)
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

func TestTransactionRepositoryCreateEnriched(t *testing.T) {
	db := newTestDB(t)
	ownerID := transactionOwner(t, db)
	repo := postgres.NewTransactionRepository(db)
	occurred := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	for _, input := range []transaction.CreateInput{
		{UserID: ownerID, AmountMinor: 300, Kind: "income", Description: "Salário", Category: "Trabalho", Installments: 1, OccurredAt: occurred},
		{UserID: ownerID, AmountMinor: 200, NecessityLevel: 2, Kind: "expense", Description: "Compra", Category: "Casa", PaymentMethod: "card", Installments: 3, OccurredAt: occurred.Add(time.Hour)},
	} {
		tx, err := transaction.NewDetailed(input)
		if err != nil {
			t.Fatal(err)
		}
		created, err := repo.Create(context.Background(), tx)
		if err != nil || created != tx {
			t.Fatalf("round trip criação: got=%+v want=%+v err=%v", created, tx, err)
		}
		var level *int
		var payment *string
		if err := db.Raw("SELECT necessity_level, payment_method FROM transactions WHERE id = ?", tx.ID).Row().Scan(&level, &payment); err != nil {
			t.Fatal(err)
		}
		if input.Kind == "income" && (level != nil || payment != nil) {
			t.Errorf("receita deve persistir NULL: level=%v payment=%v", level, payment)
		}
		if input.Kind == "expense" && (level == nil || *level != 2 || payment == nil || *payment != "card") {
			t.Errorf("despesa incorreta: level=%v payment=%v", level, payment)
		}
	}
}

func TestTransactionRepositoryDoesNotDiscardInvalidIncomeNecessity(t *testing.T) {
	db := newTestDB(t)
	owner := transactionOwner(t, db)
	tx := transaction.Transaction{
		ID: uuid.New(), UserID: owner, AmountMinor: 100, Kind: "income", NecessityLevel: 2,
		Description: "Salário", Category: "Trabalho", Installments: 1,
		OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	if _, err := postgres.NewTransactionRepository(db).Create(context.Background(), tx); err == nil {
		t.Fatal("repositório não deve descartar necessidade inválida de receita")
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

func TestEnrichedTransactionsDatabaseConstraints(t *testing.T) {
	for _, tc := range []struct {
		name         string
		kind         string
		level        any
		payment      any
		installments int
	}{
		{"unknown kind", "other", 1, nil, 1},
		{"income with necessity", "income", 2, nil, 1},
		{"expense missing necessity", "expense", nil, "card", 1},
		{"income with payment", "income", nil, "pix", 1},
		{"non-card installments", "expense", 1, "pix", 2},
		{"empty payment installments", "expense", 1, nil, 2},
		{"too many installments", "expense", 1, "card", 61},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			ownerID := transactionOwner(t, db)
			err := db.Exec(`INSERT INTO transactions
				(id, user_id, amount_minor, necessity_level, created_at, kind, description, category, payment_method, installments, occurred_at)
				VALUES (?, ?, 100, ?, ?, ?, 'Test', 'Other', ?, ?, ?)`,
				uuid.New(), ownerID, tc.level, time.Now().UTC(), tc.kind, tc.payment, tc.installments, time.Now().UTC()).Error
			if err == nil {
				t.Fatal("banco aceitou combinação inválida")
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
