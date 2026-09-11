package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestUserRepositoryListPaginatesInStableOrder(t *testing.T) {
	tx := newTestDB(t)
	repo := postgres.NewUserRepository(tx)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	users := make([]user.User, 3)
	for i := range users {
		users[i] = user.User{
			ID:   uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", 3-i)),
			Name: fmt.Sprintf("Usuário %d", i), Email: fmt.Sprintf("user%d@example.com", i),
			PasswordHash: "hash-secreto", CreatedAt: now, UpdatedAt: now.Add(time.Hour),
		}
	}
	// The oldest user has the greatest UUID; the other two share a timestamp.
	users[1].CreatedAt = now.Add(time.Minute)
	users[2].CreatedAt = now.Add(time.Minute)
	for _, i := range []int{1, 0, 2} {
		if _, err := repo.Create(ctx, users[i]); err != nil {
			t.Fatal(err)
		}
	}
	want := []user.User{users[0], users[2], users[1]}
	for _, tt := range []struct {
		limit, offset int
		want          []user.User
	}{
		{2, 0, want[:2]}, {2, 2, want[2:]}, {1, 1, want[1:2]}, {20, 3, nil},
	} {
		got, err := repo.List(ctx, user.ListInput{Limit: tt.limit, Offset: tt.offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("offset %d: esperado %d usuários, recebido %d", tt.offset, len(tt.want), len(got))
		}
		for i, u := range got {
			expected := tt.want[i]
			if u.ID != expected.ID || u.Name != expected.Name || u.Email != expected.Email || !u.CreatedAt.Equal(expected.CreatedAt) || !u.UpdatedAt.Equal(expected.UpdatedAt) {
				t.Errorf("offset %d, posição %d: dados ou ordenação incorretos", tt.offset, i)
			}
			if u.PasswordHash != "" {
				t.Error("a consulta de listagem não deve carregar hashes")
			}
		}
	}
}

func TestUserRepositoryListEmpty(t *testing.T) {
	repo := postgres.NewUserRepository(newTestDB(t))
	got, err := repo.List(context.Background(), user.ListInput{Limit: 20})
	if err != nil || len(got) != 0 {
		t.Fatalf("esperada lista vazia sem erro: %v, %v", got, err)
	}
}

func TestUserRepositoryListPreservesCanceledContext(t *testing.T) {
	repo := postgres.NewUserRepository(newTestDB(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.List(ctx, user.ListInput{Limit: 20})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperado context.Canceled, recebido %v", err)
	}
}

func TestUserRepositoryListReportsDatabaseFailure(t *testing.T) {
	tx := newTestDB(t)
	if err := tx.Exec("DROP TABLE users").Error; err != nil {
		t.Fatal(err)
	}
	_, err := postgres.NewUserRepository(tx).List(context.Background(), user.ListInput{Limit: 20})
	if err == nil {
		t.Fatal("falha de consulta não pode ser tratada como lista vazia")
	}
}
