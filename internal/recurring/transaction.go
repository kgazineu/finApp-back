package recurring

import (
	"time"

	"github.com/jmoiron/sqlx"
)

type Kind string

const (
	KindIncome  Kind = "income"
	KindExpense Kind = "expense"
)

type Transaction struct {
	ID             int64
	Description    string
	Kind           Kind
	IsFixed        bool
	Amount         int64
	StartMonth     time.Time
	IntervalMonths int
	EndMonth       *time.Time
	DayOfMonth     *int
	CreatedAt      time.Time
	ArchivedAt     *time.Time
}

type Installment struct {
	ID            int64
	TransactionID int64
	Number        int
	Amount        int64
	DueDate       time.Time
	PaidAt        *time.Time
}

type PendingInstallment struct {
	Installment
	Description string
	Kind        Kind
	IsFixed     bool
}

func (i *Installment) Overdue(now time.Time) bool {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, i.DueDate.Location())
	return i.PaidAt == nil && i.DueDate.Before(today)
}

type Occurrence struct {
	Transaction
	Date         *time.Time
	Installment  int
	Installments int
}

func monthsBetween(from, to time.Time) int {
	return (to.Year()-from.Year())*12 + int(to.Month()) - int(from.Month())
}

func (t *Transaction) OccurrenceIn(month time.Time) (Occurrence, bool) {
	diff := monthsBetween(t.StartMonth, month)
	if diff < 0 || diff%t.IntervalMonths != 0 {
		return Occurrence{}, false
	}
	if t.EndMonth != nil && monthsBetween(month, *t.EndMonth) < 0 {
		return Occurrence{}, false
	}

	o := Occurrence{Transaction: *t}

	if !t.IsFixed && t.EndMonth != nil {
		if total := monthsBetween(t.StartMonth, *t.EndMonth)/t.IntervalMonths + 1; total > 1 {
			o.Installment = diff/t.IntervalMonths + 1
			o.Installments = total
		}
	}

	if t.DayOfMonth != nil {
		lastDay := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		date := time.Date(month.Year(), month.Month(), min(*t.DayOfMonth, lastDay), 0, 0, 0, 0, time.UTC)
		o.Date = &date
	}

	return o, true
}

func (t *Transaction) InstallmentsUntil(until time.Time) []*Installment {
	var installments []*Installment

	number := 0
	for month := t.StartMonth; !month.After(until); month = month.AddDate(0, t.IntervalMonths, 0) {
		if t.EndMonth != nil && month.After(*t.EndMonth) {
			break
		}
		number++

		lastDay := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC)
		dueDate := lastDay
		if t.DayOfMonth != nil {
			dueDate = time.Date(month.Year(), month.Month(), min(*t.DayOfMonth, lastDay.Day()), 0, 0, 0, 0, time.UTC)
		}

		installments = append(installments, &Installment{
			TransactionID: t.ID,
			Number:        number,
			Amount:        t.Amount,
			DueDate:       dueDate,
		})
	}

	return installments
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
