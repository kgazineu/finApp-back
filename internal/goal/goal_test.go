package goal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
)

type repositoryStub struct {
	calls      int
	item       goal.Goal
	listInput  goal.ListInput
	userID, id uuid.UUID
	amount     int64
	err        error
}

func (r *repositoryStub) Create(_ context.Context, item goal.Goal) (goal.Goal, error) {
	r.calls++
	r.item = item
	return item, r.err
}
func (r *repositoryStub) List(_ context.Context, input goal.ListInput) ([]goal.Goal, error) {
	r.calls++
	r.listInput = input
	return []goal.Goal{r.item}, r.err
}
func (r *repositoryStub) Deposit(_ context.Context, userID, id uuid.UUID, amount int64) (goal.Goal, error) {
	r.calls++
	r.userID, r.id, r.amount = userID, id, amount
	return r.item, r.err
}
func (r *repositoryStub) Withdraw(_ context.Context, userID, id uuid.UUID, amount int64) (goal.Goal, error) {
	return r.Deposit(context.Background(), userID, id, amount)
}

func TestCreateValidationAndResult(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		input goal.CreateInput
		want  error
	}{
		{goal.CreateInput{UserID: uuid.Nil, Name: "Home", TargetMinor: 1}, goal.ErrInvalidUserID},
		{goal.CreateInput{UserID: owner, Name: " \t", TargetMinor: 1}, goal.ErrInvalidName},
		{goal.CreateInput{UserID: owner, Name: "Home", TargetMinor: 0}, goal.ErrInvalidTarget},
		{goal.CreateInput{UserID: owner, Name: "Home", TargetMinor: -1}, goal.ErrInvalidTarget},
	} {
		repo := &repositoryStub{}
		_, err := goal.NewService(repo).Create(context.Background(), tc.input)
		if !errors.Is(err, tc.want) || repo.calls != 0 {
			t.Errorf("input=%+v: err=%v calls=%d", tc.input, err, repo.calls)
		}
	}
	repo := &repositoryStub{}
	got, err := goal.NewService(repo).Create(context.Background(), goal.CreateInput{UserID: owner, Name: " Home ", TargetMinor: 100})
	if err != nil || repo.calls != 1 || got.ID == uuid.Nil || got.UserID != owner || got.Name != "Home" || got.TargetMinor != 100 || got.SavedMinor != 0 {
		t.Fatalf("create: got=%+v repo=%+v err=%v", got, repo, err)
	}
}

func TestListValidationAndDefault(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		input goal.ListInput
		want  error
	}{
		{goal.ListInput{Limit: 1}, goal.ErrInvalidUserID},
		{goal.ListInput{UserID: owner, Limit: -1}, goal.ErrInvalidPagination},
		{goal.ListInput{UserID: owner, Limit: goal.MaxListLimit + 1}, goal.ErrInvalidPagination},
		{goal.ListInput{UserID: owner, Offset: -1}, goal.ErrInvalidPagination},
	} {
		repo := &repositoryStub{}
		_, err := goal.NewService(repo).List(context.Background(), tc.input)
		if !errors.Is(err, tc.want) || repo.calls != 0 {
			t.Errorf("input=%+v: err=%v calls=%d", tc.input, err, repo.calls)
		}
	}
	repo := &repositoryStub{item: goal.Goal{ID: uuid.New()}}
	items, err := goal.NewService(repo).List(context.Background(), goal.ListInput{UserID: owner, Offset: 3})
	if err != nil || len(items) != 1 || items[0] != repo.item || repo.listInput.Limit != goal.DefaultListLimit || repo.listInput.Offset != 3 {
		t.Fatalf("list: items=%+v repo=%+v err=%v", items, repo, err)
	}
}

func TestAdjustValidationAndDelegation(t *testing.T) {
	owner, id := uuid.New(), uuid.New()
	for _, tc := range []struct {
		userID, id uuid.UUID
		amount     int64
		want       error
	}{
		{uuid.Nil, id, 1, goal.ErrInvalidUserID},
		{owner, uuid.Nil, 1, goal.ErrNotFound},
		{owner, id, 0, goal.ErrInvalidAmount},
		{owner, id, -1, goal.ErrInvalidAmount},
	} {
		for _, withdraw := range []bool{false, true} {
			repo := &repositoryStub{}
			service := goal.NewService(repo)
			var err error
			if withdraw {
				_, err = service.Withdraw(context.Background(), tc.userID, tc.id, tc.amount)
			} else {
				_, err = service.Deposit(context.Background(), tc.userID, tc.id, tc.amount)
			}
			if !errors.Is(err, tc.want) || repo.calls != 0 {
				t.Errorf("case=%+v withdraw=%t: err=%v calls=%d", tc, withdraw, err, repo.calls)
			}
		}
	}
	for _, withdraw := range []bool{false, true} {
		repo := &repositoryStub{item: goal.Goal{ID: id, SavedMinor: 42}}
		service := goal.NewService(repo)
		var got goal.Goal
		var err error
		if withdraw {
			got, err = service.Withdraw(context.Background(), owner, id, 7)
		} else {
			got, err = service.Deposit(context.Background(), owner, id, 7)
		}
		if err != nil || got != repo.item || repo.userID != owner || repo.id != id || repo.amount != 7 || repo.calls != 1 {
			t.Errorf("withdraw=%t: got=%+v repo=%+v err=%v", withdraw, got, repo, err)
		}
	}
}

func TestServicePropagatesErrorsAndCancellation(t *testing.T) {
	owner, id := uuid.New(), uuid.New()
	failure := errors.New("database failed")
	repo := &repositoryStub{err: failure}
	service := goal.NewService(repo)
	_, createErr := service.Create(context.Background(), goal.CreateInput{UserID: owner, Name: "Test", TargetMinor: 1})
	_, listErr := service.List(context.Background(), goal.ListInput{UserID: owner})
	_, depositErr := service.Deposit(context.Background(), owner, id, 1)
	_, withdrawErr := service.Withdraw(context.Background(), owner, id, 1)
	for _, err := range []error{createErr, listErr, depositErr, withdrawErr} {
		if !errors.Is(err, failure) {
			t.Errorf("lost repository error: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, run := range []func() error{
		func() error { _, err := service.Create(ctx, goal.CreateInput{}); return err },
		func() error { _, err := service.List(ctx, goal.ListInput{}); return err },
		func() error { _, err := service.Deposit(ctx, owner, id, 1); return err },
		func() error { _, err := service.Withdraw(ctx, owner, id, 1); return err },
	} {
		if err := run(); !errors.Is(err, context.Canceled) {
			t.Errorf("canceled context: %v", err)
		}
	}
	if repo.calls != 4 {
		t.Errorf("unexpected repository calls: %d", repo.calls)
	}
}
