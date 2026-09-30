package transaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type listRepositoryStub struct {
	calls int
	input transaction.ListInput
	items []transaction.Transaction
	err   error
}

func (*listRepositoryStub) Create(context.Context, transaction.Transaction) (transaction.Transaction, error) {
	panic("Create não deveria ser chamado na listagem")
}

func (r *listRepositoryStub) List(_ context.Context, input transaction.ListInput) ([]transaction.Transaction, error) {
	r.calls++
	r.input = input
	return r.items, r.err
}

func TestServiceListForOwner(t *testing.T) {
	owner := uuid.New()
	input := transaction.ListInput{UserID: owner, Limit: 20, Offset: 2}
	items := []transaction.Transaction{{ID: uuid.New(), UserID: owner}}
	repo := &listRepositoryStub{items: items}
	got, err := transaction.NewService(repo).List(context.Background(), input)
	if err != nil || repo.calls != 1 || repo.input != input || len(got) != 1 || got[0] != items[0] {
		t.Fatalf("listagem inesperada: got=%+v err=%v repo=%+v", got, err, repo)
	}
}

func TestServiceListRejectsInvalidInput(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		input transaction.ListInput
		want  error
	}{
		{transaction.ListInput{Limit: 20}, transaction.ErrInvalidUserID},
		{transaction.ListInput{UserID: owner, Limit: 0}, transaction.ErrInvalidPagination},
		{transaction.ListInput{UserID: owner, Limit: 101}, transaction.ErrInvalidPagination},
		{transaction.ListInput{UserID: owner, Limit: 20, Offset: -1}, transaction.ErrInvalidPagination},
	} {
		repo := &listRepositoryStub{}
		_, err := transaction.NewService(repo).List(context.Background(), tc.input)
		if !errors.Is(err, tc.want) || repo.calls != 0 {
			t.Errorf("entrada inválida chegou à persistência: %+v err=%v", tc.input, err)
		}
	}
}

func TestServiceListPreservesErrorsAndContext(t *testing.T) {
	failure := errors.New("db falhou")
	repo := &listRepositoryStub{err: failure}
	input := transaction.ListInput{UserID: uuid.New(), Limit: 20}
	_, err := transaction.NewService(repo).List(context.Background(), input)
	if !errors.Is(err, failure) {
		t.Fatalf("erro de banco perdido: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo.calls = 0
	_, err = transaction.NewService(repo).List(ctx, input)
	if !errors.Is(err, context.Canceled) || repo.calls != 0 {
		t.Fatalf("contexto cancelado ignorado: %v", err)
	}
}
