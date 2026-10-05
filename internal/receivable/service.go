package receivable

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound           = errors.New("installment not found")
	ErrReceivableNotFound = errors.New("receivable not found")
	ErrInvalidReceivable  = errors.New("invalid receivable")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, rec *Receivable, installments int, firstDueDate time.Time) (*Receivable, error) {
	if rec.Kind == KindSplit && (rec.InterestRate != 0 || installments != 1) {
		return nil, fmt.Errorf("%w: split cannot have interest or more than one installment", ErrInvalidReceivable)
	}

	rec.Installments = buildInstallments(rec.Amount, rec.InterestRate, installments, firstDueDate)

	return s.repo.Create(ctx, userID, rec)
}

func (s *Service) FindAll(ctx context.Context, userID uuid.UUID) ([]*Receivable, error) {
	return s.repo.FindAll(ctx, userID)
}

func (s *Service) Update(ctx context.Context, userID uuid.UUID, id int64, debtor, description string) (*Receivable, error) {
	return s.repo.Update(ctx, userID, id, debtor, description)
}

func (s *Service) Delete(ctx context.Context, userID uuid.UUID, id int64) (bool, error) {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) FindPendingInstallments(ctx context.Context, userID uuid.UUID) ([]*PendingInstallment, error) {
	return s.repo.FindPendingInstallments(ctx, userID)
}

func (s *Service) FindPendingUntil(ctx context.Context, userID uuid.UUID, until time.Time) ([]*PendingInstallment, error) {
	return s.repo.FindPendingUntil(ctx, userID, until)
}

func (s *Service) FindPaidInstallments(ctx context.Context, userID uuid.UUID, since time.Time) ([]*PendingInstallment, error) {
	return s.repo.FindPaidInstallments(ctx, userID, since)
}

func (s *Service) SetInstallmentPaid(ctx context.Context, userID uuid.UUID, id int64, paid bool) (*Installment, error) {
	return s.repo.SetInstallmentPaid(ctx, userID, id, paid)
}

func buildInstallments(amount int64, interestRate, n int, firstDueDate time.Time) []*Installment {
	total := amount + amount*int64(interestRate)/100
	base := total / int64(n)

	installments := make([]*Installment, n)
	for i := range installments {
		installments[i] = &Installment{
			Number:  i + 1,
			Amount:  base,
			DueDate: addMonths(firstDueDate, i), // sempre a partir da 1ª data, nunca da anterior
		}
	}
	installments[n-1].Amount += total - base*int64(n)

	return installments
}

// addMonths soma meses sem transbordar: 31/10 + 1 mês = 30/11 (o AddDate daria 01/12).
func addMonths(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, t.Location())
	lastDay := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), lastDay)-1)
}
