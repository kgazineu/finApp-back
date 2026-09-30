package postgres_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

func goalOwner(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	owner, err := user.New("Owner", uuid.NewString()+"@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewUserRepository(db).Create(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	return owner.ID
}

func goalCreate(t *testing.T, service *goal.Service, owner uuid.UUID, name string, target int64) goal.Goal {
	t.Helper()
	item, err := service.Create(context.Background(), goal.CreateInput{UserID: owner, Name: name, TargetMinor: target})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestGoalCreateListAndOwnership(t *testing.T) {
	db, _ := testutil.Database(t)
	first, second := goalOwner(t, db), goalOwner(t, db)
	service := goal.NewService(postgres.NewGoalRepository(db))
	a := goalCreate(t, service, first, "  Vacation  ", 300)
	b := goalCreate(t, service, first, "Home", 400)
	other := goalCreate(t, service, second, "Private", 500)
	if a.ID == uuid.Nil || a.UserID != first || a.Name != "Vacation" || a.TargetMinor != 300 || a.SavedMinor != 0 || a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() {
		t.Fatalf("unexpected created goal: %+v", a)
	}
	// Set deterministic timestamps and tie-break with descending IDs.
	stamp := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Exec("UPDATE goals SET created_at = ? WHERE id IN (?, ?)", stamp, a.ID, b.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE goals SET created_at = ? WHERE id = ?", stamp.Add(time.Hour), other.ID).Error; err != nil {
		t.Fatal(err)
	}
	ordered := []uuid.UUID{a.ID, b.ID}
	if ordered[0].String() < ordered[1].String() {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	for _, tc := range []struct {
		input goal.ListInput
		want  []uuid.UUID
	}{
		{goal.ListInput{UserID: first, Limit: 1}, ordered[:1]},
		{goal.ListInput{UserID: first, Limit: 1, Offset: 1}, ordered[1:]},
		{goal.ListInput{UserID: first, Offset: 2}, nil},
		{goal.ListInput{UserID: second}, []uuid.UUID{other.ID}},
	} {
		got, err := service.List(context.Background(), tc.input)
		if err != nil || len(got) != len(tc.want) {
			t.Fatalf("input=%+v: goals=%+v err=%v", tc.input, got, err)
		}
		for i, item := range got {
			if item.ID != tc.want[i] || item.UserID != tc.input.UserID {
				t.Fatalf("list leaked or reordered: %+v", got)
			}
		}
	}
	for _, operation := range []func(uuid.UUID, uuid.UUID, int64) (goal.Goal, error){
		func(owner, id uuid.UUID, amount int64) (goal.Goal, error) {
			return service.Deposit(context.Background(), owner, id, amount)
		},
		func(owner, id uuid.UUID, amount int64) (goal.Goal, error) {
			return service.Withdraw(context.Background(), owner, id, amount)
		},
	} {
		for _, id := range []uuid.UUID{a.ID, uuid.New()} {
			_, err := operation(second, id, 1)
			if !errors.Is(err, goal.ErrNotFound) {
				t.Errorf("other owner's goal or missing goal: %v", err)
			}
		}
	}
}

func TestGoalAtomicBalancesAndGuards(t *testing.T) {
	db, _ := testutil.Database(t)
	owner := goalOwner(t, db)
	service := goal.NewService(postgres.NewGoalRepository(db))
	item := goalCreate(t, service, owner, "Emergency", 100)
	added, err := service.Deposit(context.Background(), owner, item.ID, 90)
	if err != nil || added.SavedMinor != 90 || added.TargetMinor != item.TargetMinor || added.CreatedAt != item.CreatedAt || added.UpdatedAt.IsZero() {
		t.Fatalf("deposit: %+v err=%v", added, err)
	}
	removed, err := service.Withdraw(context.Background(), owner, item.ID, 40)
	if err != nil || removed.SavedMinor != 50 {
		t.Fatalf("withdraw: %+v err=%v", removed, err)
	}
	if _, err := service.Withdraw(context.Background(), owner, item.ID, 51); !errors.Is(err, goal.ErrInsufficientFunds) {
		t.Fatalf("expected insufficient funds: %v", err)
	}
	maxed := goalCreate(t, service, owner, "Maximum", math.MaxInt64)
	if got, err := service.Deposit(context.Background(), owner, maxed.ID, math.MaxInt64); err != nil || got.SavedMinor != math.MaxInt64 {
		t.Fatalf("deposit max: %+v err=%v", got, err)
	}
	if _, err := service.Deposit(context.Background(), owner, maxed.ID, 1); !errors.Is(err, goal.ErrInvalidAmount) {
		t.Fatalf("expected overflow rejection: %v", err)
	}
	if got, err := service.Withdraw(context.Background(), owner, maxed.ID, math.MaxInt64); err != nil || got.SavedMinor != 0 {
		t.Fatalf("withdraw max: %+v err=%v", got, err)
	}
	rows, err := service.List(context.Background(), goal.ListInput{UserID: owner})
	if err != nil || len(rows) != 2 {
		t.Fatalf("list balances: %+v err=%v", rows, err)
	}
	for _, row := range rows {
		if row.ID == item.ID && row.SavedMinor != 50 || row.ID == maxed.ID && row.SavedMinor != 0 {
			t.Fatalf("guard changed balance: %+v", row)
		}
	}
}

func TestGoalConcurrentUpdates(t *testing.T) {
	db, _ := testutil.Database(t)
	owner := goalOwner(t, db)
	service := goal.NewService(postgres.NewGoalRepository(db))
	item := goalCreate(t, service, owner, "Concurrent", 1)
	const workers = 12
	run := func(operation func() error) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- operation() }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	run(func() error { _, err := service.Deposit(context.Background(), owner, item.ID, 1); return err })
	run(func() error { _, err := service.Withdraw(context.Background(), owner, item.ID, 1); return err })
	// Only one competing withdrawal of a single saved unit may succeed.
	if _, err := service.Deposit(context.Background(), owner, item.ID, 1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, insufficient := 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Withdraw(context.Background(), owner, item.ID, 1)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, goal.ErrInsufficientFunds):
				insufficient++
			default:
				t.Errorf("unexpected withdrawal error: %v", err)
			}
		}()
	}
	wg.Wait()
	if success != 1 || insufficient != workers-1 {
		t.Fatalf("concurrent withdrawal results: success=%d insufficient=%d", success, insufficient)
	}
	var balance int64
	if err := db.Raw("SELECT saved_minor FROM goals WHERE id = ?", item.ID).Row().Scan(&balance); err != nil || balance != 0 {
		t.Fatalf("final saved balance=%d err=%v", balance, err)
	}
}

func TestGoalsDatabaseConstraints(t *testing.T) {
	db, _ := testutil.Database(t)
	owner := goalOwner(t, db)
	for _, tc := range []struct {
		name          string
		userID        uuid.UUID
		target, saved int64
	}{
		{"Valid", uuid.New(), 1, 0}, // foreign key
		{" ", owner, 1, 0},
		{"Valid", owner, 0, 0},
		{"Valid", owner, -1, 0},
		{"Valid", owner, 1, -1},
	} {
		err := db.Exec(`INSERT INTO goals (id, user_id, name, target_minor, saved_minor)
			VALUES (?, ?, ?, ?, ?)`, uuid.New(), tc.userID, tc.name, tc.target, tc.saved).Error
		if err == nil {
			t.Errorf("accepted invalid row: %+v", tc)
		}
	}
	item := goalCreate(t, goal.NewService(postgres.NewGoalRepository(db)), owner, "Valid", 1)
	if err := db.Exec("UPDATE goals SET saved_minor = -1 WHERE id = ?", item.ID).Error; err == nil {
		t.Fatal("accepted negative balance update")
	}
}

func TestGoalRepositoryCanceledContext(t *testing.T) {
	db, _ := testutil.Database(t)
	repo := postgres.NewGoalRepository(db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	owner, id := uuid.New(), uuid.New()
	_, createErr := repo.Create(ctx, goal.Goal{ID: id, UserID: owner, Name: "x", TargetMinor: 1})
	_, listErr := repo.List(ctx, goal.ListInput{UserID: owner, Limit: 20})
	_, depositErr := repo.Deposit(ctx, owner, id, 1)
	_, withdrawErr := repo.Withdraw(ctx, owner, id, 1)
	for _, err := range []error{createErr, listErr, depositErr, withdrawErr} {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("canceled context lost: %v", err)
		}
	}
}
