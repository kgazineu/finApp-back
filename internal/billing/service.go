package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/account"
	"github.com/kgazineu/finApp-back/internal/receivable"
	"github.com/kgazineu/finApp-back/internal/recurring"
)

var ErrInvalidEntries = errors.New("lançamentos inválidos")

type accountFinder interface {
	FindActive(ctx context.Context, userID uuid.UUID) ([]*account.Account, error)
}

type Service struct {
	repo         *Repository
	accounts     accountFinder
	transactions *recurring.Service
	receivables  *receivable.Service
}

func NewService(repo *Repository, accounts accountFinder, transactions *recurring.Service, receivables *receivable.Service) *Service {
	return &Service{repo: repo, accounts: accounts, transactions: transactions, receivables: receivables}
}

type MonthProjection struct {
	Date         time.Time
	Total        int64
	Transactions []*recurring.PendingInstallment
	Receivables  []*receivable.PendingInstallment
}

func (s *Service) FindMany(ctx context.Context, userID uuid.UUID) ([]*BillingRegistration, error) {
	return s.repo.FindManyBillingRegistrations(ctx, userID)
}

func (s *Service) MonthProjection(ctx context.Context, userID uuid.UUID, now time.Time, months int) (*MonthProjection, error) {
	y, m, _ := now.Date()
	endOfMonth := time.Date(y, m+time.Month(months), 0, 0, 0, 0, 0, time.UTC)

	p := &MonthProjection{Date: endOfMonth.AddDate(0, 0, 1)}

	last, err := s.repo.findLastBillingRegistration(ctx, userID)
	if err != nil {
		return nil, err
	}
	if last != nil {
		p.Total = last.total
	}

	p.Transactions, err = s.transactions.FindPendingInstallments(ctx, userID, endOfMonth)
	if err != nil {
		return nil, err
	}
	for _, i := range p.Transactions {
		if i.Kind == recurring.KindIncome {
			p.Total += i.Amount
		} else {
			p.Total -= i.Amount
		}
	}

	p.Receivables, err = s.receivables.FindPendingUntil(ctx, userID, endOfMonth)
	if err != nil {
		return nil, err
	}
	for _, i := range p.Receivables {
		p.Total += i.Amount
	}

	return p, nil
}

func (s *Service) CreateBillingRegistration(ctx context.Context, userID uuid.UUID, input []CreateBillingEntryRequest) (*BillingRegistration, error) {
	accounts, err := s.accounts.FindActive(ctx, userID)
	if err != nil {
		return nil, err
	}

	amounts := make(map[int64]int64, len(input))
	for _, e := range input {
		amounts[e.AccountID] = *e.Amount
	}

	if len(amounts) != len(accounts) {
		return nil, fmt.Errorf("%w: era esperado um saldo para cada uma das %d contas ativas, mas vieram %d", ErrInvalidEntries, len(accounts), len(amounts))
	}

	reg := &BillingRegistration{}
	kinds := make(map[int64]account.Kind, len(accounts))
	for _, acc := range accounts {
		amount, ok := amounts[acc.ID]
		if !ok {
			return nil, fmt.Errorf("%w: falta o saldo da conta %s", ErrInvalidEntries, acc.Name)
		}
		reg.entries = append(reg.entries, &BillingEntry{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Amount:      amount,
		})
		kinds[acc.ID] = acc.Kind
	}

	last, err := s.repo.findLastBillingRegistration(ctx, userID)
	if err != nil {
		return nil, err
	}

	calculate(reg, kinds, last)

	return s.repo.createBillingRegistration(ctx, userID, reg)
}

func calculate(reg *BillingRegistration, kinds map[int64]account.Kind, last *BillingRegistration) {
	reg.total = 0
	for _, e := range reg.entries {
		switch kind := kinds[e.AccountID]; kind {
		case account.KindLiability:
			reg.total -= e.Amount
		case account.KindAsset:
			reg.total += e.Amount
		default:
			return
		}
	}

	reg.delta = nil // primeiro registro: delta só faz sentido em relação a um anterior
	if last != nil {
		delta := reg.total - last.total
		reg.delta = &delta
	}
}
