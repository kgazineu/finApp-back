package recurring

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("transação não encontrada")
	ErrInstallmentNotFound = errors.New("parcela não encontrada")
	ErrInvalidTransaction  = errors.New("transação inválida")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, t *Transaction, installments int) (*Transaction, error) {
	t.EndMonth = endMonthFor(t, installments)

	return s.repo.Create(ctx, userID, t)
}

// sem "quantas vezes": variável acontece uma vez só, fixa não tem fim
func endMonthFor(t *Transaction, installments int) *time.Time {
	if installments == 0 && !t.IsFixed {
		installments = 1
	}
	if installments == 0 {
		return nil
	}

	endMonth := t.StartMonth.AddDate(0, (installments-1)*t.IntervalMonths, 0)
	return &endMonth
}

func (s *Service) FindByMonth(ctx context.Context, userID uuid.UUID, month time.Time) ([]Occurrence, error) {
	transactions, err := s.repo.FindAll(ctx, userID)
	if err != nil {
		return nil, err
	}

	var occurrences []Occurrence
	for _, t := range transactions {
		if o, ok := t.OccurrenceIn(month); ok {
			occurrences = append(occurrences, o)
		}
	}

	return occurrences, nil
}

func (s *Service) FindPendingInstallments(ctx context.Context, userID uuid.UUID, until time.Time) ([]*PendingInstallment, error) {
	if err := s.syncInstallments(ctx, userID, until); err != nil {
		return nil, err
	}

	return s.repo.FindPendingInstallments(ctx, userID, until)
}

func (s *Service) syncInstallments(ctx context.Context, userID uuid.UUID, until time.Time) error {
	transactions, err := s.repo.FindAll(ctx, userID)
	if err != nil {
		return err
	}

	var installments []*Installment
	for _, t := range transactions {
		// meses antes de você cadastrar a transaction não viram parcela pendente
		y, m, _ := t.CreatedAt.In(time.Local).Date()
		createdMonth := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)

		installments = append(installments, t.InstallmentsBetween(createdMonth, until)...)
	}

	return s.repo.InsertInstallments(ctx, installments)
}

func (s *Service) FindPaidInstallments(ctx context.Context, userID uuid.UUID, since time.Time) ([]*PendingInstallment, error) {
	return s.repo.FindPaidInstallments(ctx, userID, since)
}

func (s *Service) SetInstallmentPaid(ctx context.Context, userID uuid.UUID, id int64, paid bool) (*Installment, error) {
	return s.repo.SetInstallmentPaid(ctx, userID, id, paid)
}

func (s *Service) FindAll(ctx context.Context, userID uuid.UUID) ([]*Transaction, error) {
	return s.repo.FindAll(ctx, userID)
}

func (s *Service) Update(ctx context.Context, userID uuid.UUID, t *Transaction) (*Transaction, error) {
	return s.repo.Update(ctx, userID, t)
}

func (s *Service) Delete(ctx context.Context, userID uuid.UUID, id int64) (bool, error) {
	return s.repo.Delete(ctx, userID, id)
}
