package overview_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/overview"
)

type stubRepository struct {
	calls int
	user  uuid.UUID
	start time.Time
	end   time.Time
	value overview.Snapshot
	err   error
}

func (r *stubRepository) Fetch(_ context.Context, user uuid.UUID, start, end time.Time) (overview.Snapshot, error) {
	r.calls++
	r.user, r.start, r.end = user, start, end
	return r.value, r.err
}

func TestServiceValidationAndContext(t *testing.T) {
	repo := &stubRepository{}
	service := overview.NewService(repo)
	month := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		ctx   context.Context
		user  uuid.UUID
		month time.Time
		want  error
	}{
		{"nil user", context.Background(), uuid.Nil, month, overview.ErrInvalidUserID},
		{"zero month", context.Background(), uuid.New(), time.Time{}, overview.ErrInvalidMonth},
		{"canceled", canceledContext(), uuid.New(), month, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Get(tc.ctx, tc.user, tc.month); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
	if repo.calls != 0 {
		t.Fatalf("invalid requests called repository %d times", repo.calls)
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestServiceNormalizesUTCMonthAndPropagatesError(t *testing.T) {
	repo := &stubRepository{value: overview.Snapshot{IncomeMinor: 42}}
	service := overview.NewService(repo)
	user := uuid.New()
	// Local April 1 is still March in UTC.
	month := time.Date(2026, time.April, 1, 0, 30, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	got, err := service.Get(context.Background(), user, month)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	if repo.calls != 1 || repo.user != user || !repo.start.Equal(start) || repo.start.Location() != time.UTC ||
		!repo.end.Equal(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)) ||
		got.Month != start || got.IncomeMinor != 42 {
		t.Fatalf("repo=%+v snapshot=%+v", repo, got)
	}
	repo.err = errors.New("storage failure")
	if _, err := service.Get(context.Background(), user, month); !errors.Is(err, repo.err) {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
}
