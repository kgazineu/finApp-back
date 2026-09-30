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

func TestServiceGetReturnsPublicProfile(t *testing.T) {
	id := uuid.New()
	ctx := context.Background()
	repo := &repositoryWithFindStub{find: func(gotCtx context.Context, gotID uuid.UUID) (user.User, error) {
		if gotCtx != ctx || gotID != id {
			t.Error("Get must pass the caller context and ID to FindByID")
		}
		return user.User{ID: id, Name: "Kaian", Email: "kaian@example.com", PasswordHash: "secret-hash"}, nil
	}}
	got, err := user.NewService(repo, nil).Get(ctx, id)
	if err != nil || got.ID != id || got.Name != "Kaian" || got.Email != "kaian@example.com" || got.PasswordHash != "" {
		t.Fatalf("unexpected public profile: %+v, %v", got, err)
	}
}

type repositoryWithFindStub struct {
	repositoryStub
	find func(context.Context, uuid.UUID) (user.User, error)
}

func (r *repositoryWithFindStub) FindByID(ctx context.Context, id uuid.UUID) (user.User, error) {
	return r.find(ctx, id)
}

func TestServiceGetRejectsInvalidIDAndCanceledContext(t *testing.T) {
	repo := &repositoryWithFindStub{find: func(context.Context, uuid.UUID) (user.User, error) {
		t.Fatal("FindByID must not be called")
		return user.User{}, nil
	}}
	service := user.NewService(repo, nil)
	if _, err := service.Get(context.Background(), uuid.Nil); !errors.Is(err, user.ErrInvalidUserID) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Get(ctx, uuid.New()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

func TestServiceGetPreservesRepositoryError(t *testing.T) {
	failure := errors.New("database failure")
	repo := &repositoryWithFindStub{find: func(context.Context, uuid.UUID) (user.User, error) {
		return user.User{}, failure
	}}
	if _, err := user.NewService(repo, nil).Get(context.Background(), uuid.New()); !errors.Is(err, failure) || err == failure {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
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
