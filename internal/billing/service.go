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

// projectionWindow: a projeção é para o dia 1 do N-ésimo mês depois do atual, e tudo que vence até o
// fim desse mês conta como pago nesse dia 1. Em outubro, "1 mês" é 01/11 com novembro inteiro.
func projectionWindow(now time.Time, months int) (projectedFor, until time.Time) {
	y, m, _ := now.Date()
	projectedFor = time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	return projectedFor, projectedFor.AddDate(0, 1, -1)
}

func (s *Service) MonthProjection(ctx context.Context, userID uuid.UUID, now time.Time, months int) (*MonthProjection, error) {
	projectedFor, until := projectionWindow(now, months)

	p := &MonthProjection{Date: projectedFor}

	last, err := s.repo.findLastBillingRegistration(ctx, userID)
	if err != nil {
		return nil, err
	}
	if last != nil {
		p.Total = last.total
	}

	p.Transactions, err = s.transactions.FindPendingInstallments(ctx, userID, until)
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

	p.Receivables, err = s.receivables.FindPendingUntil(ctx, userID, until)
	if err != nil {
		return nil, err
	}
	for _, i := range p.Receivables {
		p.Total += i.Amount
	}

	return p, nil
}

// MonthlyGrowth dá a noção de quanto você cresce por mês: no mês que vem, entradas + valores a receber −
// despesas fixas. Vem das regras cadastradas, pagas ou não (marcar algo adiantado não muda o número);
// despesas variáveis (compras, parcelamentos) ficam de fora.
// ponytail: só o mês que vem; média de vários meses se despesas anuais (intervalo > 1) distorcerem
func (s *Service) MonthlyGrowth(ctx context.Context, userID uuid.UUID, now time.Time) (month time.Time, growth int64, err error) {
	y, m, _ := now.Date()
	month = time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)

	occurrences, err := s.transactions.FindByMonth(ctx, userID, month)
	if err != nil {
		return month, 0, err
	}
	for _, o := range occurrences {
		switch {
		case o.Kind == recurring.KindIncome:
			growth += o.Amount
		case o.IsFixed:
			growth -= o.Amount
		}
	}

	receivables, err := s.receivables.SumDueBetween(ctx, userID, month, month.AddDate(0, 1, -1))
	return month, growth + receivables, err
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
