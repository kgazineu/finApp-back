package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type repositoryStub struct {
	called   int
	received transaction.Transaction
	result   transaction.Transaction
	err      error
}

func (r *repositoryStub) List(context.Context, transaction.ListInput) ([]transaction.Transaction, error) {
	panic("List não deveria ser chamado nos testes de criação")
}

func (r *repositoryStub) Create(_ context.Context, tx transaction.Transaction) (transaction.Transaction, error) {
	r.called++
	r.received = tx
	if r.err != nil {
		return transaction.Transaction{}, r.err
	}
	if r.result.ID != uuid.Nil {
		return r.result, nil
	}
	return tx, nil
}

func TestServiceCreatePersistsAndReturnsRepositoryResult(t *testing.T) {
	persisted := transaction.Transaction{ID: uuid.New(), UserID: uuid.New(), AmountMinor: 999, NecessityLevel: 4}
	repo := &repositoryStub{result: persisted}
	service := transaction.NewService(repo)
	ownerID := uuid.New()
	created, err := service.Create(context.Background(), transaction.CreateInput{
		UserID: ownerID, AmountMinor: 100, NecessityLevel: 3,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if repo.called != 1 || repo.received.UserID != ownerID || repo.received.AmountMinor != 100 || repo.received.NecessityLevel != 3 {
		t.Fatalf("repositório deve receber uma transação válida uma vez: %+v", repo)
	}
	if created != persisted {
		t.Errorf("esperado retorno da persistência, recebido %+v", created)
	}
}

func TestServiceCreateRejectsInvalidInputWithoutPersisting(t *testing.T) {
	for _, input := range []transaction.CreateInput{
		{UserID: uuid.Nil, AmountMinor: 100, NecessityLevel: 1},
		{UserID: uuid.New(), AmountMinor: 0, NecessityLevel: 1},
		{UserID: uuid.New(), AmountMinor: 100, NecessityLevel: 6},
	} {
		repo := &repositoryStub{}
		_, err := transaction.NewService(repo).Create(context.Background(), input)
		if err == nil || repo.called != 0 {
			t.Errorf("entrada inválida persistida: input=%+v err=%v", input, err)
		}
	}
}

func TestServiceCreateEnrichedIncome(t *testing.T) {
	repo := &repositoryStub{}
	input := transaction.CreateInput{
		UserID: uuid.New(), AmountMinor: 4500, Kind: "income", Description: "  Salário ",
		Category: " Trabalho ", Installments: 1,
	}
	input.OccurredAt = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	got, err := transaction.NewService(repo).Create(context.Background(), input)
	if err != nil || repo.called != 1 || got != repo.received || got.Kind != "income" || got.NecessityLevel != 0 ||
		got.Description != "Salário" || got.Category != "Trabalho" || !got.OccurredAt.Equal(input.OccurredAt) {
		t.Fatalf("criação enriquecida inesperada: got=%+v err=%v repo=%+v", got, err, repo)
	}
}

func TestServiceCreateRejectsPartialEnrichment(t *testing.T) {
	repo := &repositoryStub{}
	_, err := transaction.NewService(repo).Create(context.Background(), transaction.CreateInput{
		UserID: uuid.New(), AmountMinor: 100, NecessityLevel: 1, Description: "Compra",
	})
	if !errors.Is(err, transaction.ErrInvalidKind) || repo.called != 0 {
		t.Fatalf("entrada parcialmente enriquecida deveria falhar: %v", err)
	}
}

func TestServiceCreatePropagatesRepositoryError(t *testing.T) {
	failure := errors.New("falha na persistência")
	repo := &repositoryStub{err: failure}
	_, err := transaction.NewService(repo).Create(context.Background(), transaction.CreateInput{
		UserID: uuid.New(), AmountMinor: 100, NecessityLevel: 2,
	})
	if !errors.Is(err, failure) || repo.called != 1 {
		t.Errorf("falha do repositório não propagada: %v", err)
	}
}

func TestServiceCreateHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &repositoryStub{}
	_, err := transaction.NewService(repo).Create(ctx, transaction.CreateInput{
		UserID: uuid.New(), AmountMinor: 100, NecessityLevel: 2,
	})
	if !errors.Is(err, context.Canceled) || repo.called != 0 {
		t.Errorf("contexto cancelado ignorado: %v", err)
	}
}
