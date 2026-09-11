package user_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

type listRepositoryStub struct {
	repositoryStub
	list func(context.Context, user.ListInput) ([]user.User, error)
}

func (r *listRepositoryStub) List(ctx context.Context, input user.ListInput) ([]user.User, error) {
	return r.list(ctx, input)
}

func TestServiceListReturnsRepositoryUsers(t *testing.T) {
	want := []user.User{{ID: uuid.New(), Name: "Kaian", Email: "kaian@example.com"}}
	input := user.ListInput{Limit: 10, Offset: 20}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	repo := &listRepositoryStub{list: func(gotCtx context.Context, got user.ListInput) ([]user.User, error) {
		called = true
		if got != input || gotCtx != ctx {
			t.Error("o repositório deve receber a paginação e o contexto do serviço")
		}
		return want, nil
	}}
	got, err := user.NewService(repo, nil).List(ctx, input)
	if err != nil || !called || !reflect.DeepEqual(got, want) {
		t.Fatalf("listagem inesperada: users=%v, err=%v, called=%v", got, err, called)
	}
}

func TestServiceListRejectsInvalidPagination(t *testing.T) {
	for _, input := range []user.ListInput{
		{Limit: 0}, {Limit: -1}, {Limit: 101}, {Limit: 20, Offset: -1},
	} {
		repo := &listRepositoryStub{list: func(context.Context, user.ListInput) ([]user.User, error) {
			t.Fatal("paginação inválida não deve chegar ao repositório")
			return nil, nil
		}}
		_, err := user.NewService(repo, nil).List(context.Background(), input)
		if !errors.Is(err, user.ErrInvalidPagination) {
			t.Errorf("entrada %+v: esperado ErrInvalidPagination, recebido %v", input, err)
		}
	}
}

func TestServiceListPreservesRepositoryError(t *testing.T) {
	want := errors.New("falha no banco")
	repo := &listRepositoryStub{list: func(context.Context, user.ListInput) ([]user.User, error) {
		return nil, want
	}}
	_, err := user.NewService(repo, nil).List(context.Background(), user.ListInput{Limit: 100})
	if !errors.Is(err, want) {
		t.Fatalf("esperado preservar erro do repositório, recebido %v", err)
	}
}

func TestServiceListStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &listRepositoryStub{list: func(context.Context, user.ListInput) ([]user.User, error) {
		t.Fatal("contexto cancelado não deve chegar ao repositório")
		return nil, nil
	}}
	_, err := user.NewService(repo, nil).List(ctx, user.ListInput{Limit: 20})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperado context.Canceled, recebido %v", err)
	}
}
