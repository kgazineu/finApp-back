package billing

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/receivable"
	"github.com/kgazineu/finApp-back/internal/recurring"
)

type Module struct {
	controller *Controller
}

type BillingRegistration struct {
	ID        int64
	delta     int64
	total     int64
	entries   []*BillingEntry
	CreatedAt time.Time
}

type BillingEntry struct {
	ID                    int64
	BillingRegistrationID int64
	AccountID             int64
	AccountName           string
	Amount                int64
}

func NewModule(db *sqlx.DB, accounts accountFinder, transactions *recurring.Service, receivables *receivable.Service) *Module {
	repo := NewRepository(db)
	service := NewService(repo, accounts, transactions, receivables)
	controller := NewController(service)
	return &Module{controller: controller}
}
