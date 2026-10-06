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

// AmountMode diz como o valor informado vira parcelas.
type AmountMode string

const (
	AmountTotal       AmountMode = "total"       // valor total, dividido entre as parcelas
	AmountInstallment AmountMode = "installment" // valor de cada parcela (ex.: assinatura mensal)
)

type Receivable struct {
	ID           int64
	Kind         Kind
	Debtor       string
	Description  string
	Amount       int64 // sem juros: o total (AmountTotal) ou o de cada parcela (AmountInstallment)
	AmountMode   AmountMode
	InterestRate int // 5 = 5%; só no modo total
	Installments []*Installment
	CreatedAt    time.Time
}

// InstallmentChanges edita uma parcela. ApplyToFollowing leva o novo valor e o novo dia de
// vencimento (mês a mês a partir da nova data) para as parcelas seguintes ainda não recebidas.
type InstallmentChanges struct {
	Paid             *bool
	Amount           *int64
	DueDate          *time.Time
	ApplyToFollowing bool
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
