package receivable

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound           = errors.New("parcela não encontrada")
	ErrReceivableNotFound = errors.New("valor a receber não encontrado")
	ErrInvalidReceivable  = errors.New("valor a receber inválido")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, rec *Receivable, installments int, firstDueDate time.Time) (*Receivable, error) {
	if rec.Kind == KindSplit && (rec.InterestRate != 0 || installments != 1) {
		return nil, fmt.Errorf("%w: conta dividida não pode ter juros nem mais de uma parcela", ErrInvalidReceivable)
	}
	if rec.AmountMode == "" {
		rec.AmountMode = AmountTotal
	}
	if rec.AmountMode == AmountInstallment && rec.InterestRate != 0 {
		return nil, fmt.Errorf("%w: com valor por parcela não há juros para calcular; informe o valor de cada parcela já com os juros", ErrInvalidReceivable)
	}

	rec.Installments = buildInstallments(rec.Amount, rec.AmountMode, rec.InterestRate, installments, firstDueDate)

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

func (s *Service) SumDueBetween(ctx context.Context, userID uuid.UUID, from, to time.Time) (int64, error) {
	return s.repo.SumDueBetween(ctx, userID, from, to)
}

func (s *Service) FindPaidInstallments(ctx context.Context, userID uuid.UUID, since time.Time) ([]*PendingInstallment, error) {
	return s.repo.FindPaidInstallments(ctx, userID, since)
}

func (s *Service) UpdateInstallment(ctx context.Context, userID uuid.UUID, id int64, changes InstallmentChanges) (*Installment, error) {
	if changes.Paid == nil && changes.Amount == nil && changes.DueDate == nil {
		return nil, fmt.Errorf("%w: informe o que alterar na parcela (recebida, valor ou vencimento)", ErrInvalidReceivable)
	}
	return s.repo.UpdateInstallment(ctx, userID, id, changes)
}

func buildInstallments(amount int64, mode AmountMode, interestRate, n int, firstDueDate time.Time) []*Installment {
	// valor por parcela: cada uma vale exatamente o informado
	total := amount * int64(n)
	if mode == AmountTotal {
		total = amount + amount*int64(interestRate)/100
	}
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
