package receivable

import (
	"time"

	"github.com/jmoiron/sqlx"
)

type Kind string

const (
	KindSplit Kind = "split"
	KindLoan  Kind = "loan"
)

type Receivable struct {
	ID           int64
	Kind         Kind
	Debtor       string
	Description  string
	Amount       int64 // valor sem juros
	InterestRate int   // 5 = 5%
	Installments []*Installment
	CreatedAt    time.Time
}

type Installment struct {
	ID           int64
	ReceivableID int64
	Number       int
	Amount       int64 // já com juros
	DueDate      time.Time
	PaidAt       *time.Time
}

type PendingInstallment struct {
	Installment
	Debtor      string
	Description string
}

func (i *Installment) Overdue(now time.Time) bool {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, i.DueDate.Location())
	return i.PaidAt == nil && i.DueDate.Before(today)
}

type Module struct {
	controller *Controller
	service    *Service
}

func NewModule(db *sqlx.DB) *Module {
	repo := NewRepository(db)
	service := NewService(repo)
	controller := NewController(service)
	return &Module{controller: controller, service: service}
}

func (m *Module) Service() *Service {
	return m.service
}
