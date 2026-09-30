package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/overview"
)

type OverviewService interface {
	Get(ctx context.Context, userID uuid.UUID, month time.Time) (overview.Snapshot, error)
}
