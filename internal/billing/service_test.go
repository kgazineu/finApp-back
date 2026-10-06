package billing

import (
	"testing"
	"time"

	"github.com/kgazineu/finApp-back/internal/account"
)

func TestProjectionWindow(t *testing.T) {
	tests := []struct {
		name, now string
		months    int
		want      string
	}{
		{"1 mês: dia 1 do mês que vem, com ele inteiro", "2026-10-06", 1, "2026-11-01 até 2026-11-30"},
		{"3 meses: salários de novembro, dezembro e janeiro", "2026-10-06", 3, "2027-01-01 até 2027-01-31"},
		{"virada do ano", "2026-12-31", 1, "2027-01-01 até 2027-01-31"},
		{"fevereiro", "2027-01-31", 1, "2027-02-01 até 2027-02-28"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now, _ := time.Parse(time.DateOnly, tt.now)
			projectedFor, until := projectionWindow(now, tt.months)
			if got := projectedFor.Format(time.DateOnly) + " até " + until.Format(time.DateOnly); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCalculate(t *testing.T) {
	kinds := map[int64]account.Kind{
		1: account.KindAsset,
		2: account.KindAsset,
		3: account.KindLiability,
	}

	newReg := func(inter, mp, fatura int64) *BillingRegistration {
		return &BillingRegistration{entries: []*BillingEntry{
			{AccountID: 1, Amount: inter},
			{AccountID: 2, Amount: mp},
			{AccountID: 3, Amount: fatura},
		}}
	}

	first := newReg(0, 10000, 5000)
	calculate(first, kinds, nil)
	if first.total != 5000 || first.delta != nil {
		t.Fatalf("first: total=%d delta=%v (o primeiro registro não tem delta)", first.total, first.delta)
	}

	second := newReg(0, 10050, 5500)
	calculate(second, kinds, first)
	if second.total != 4550 || second.delta == nil || *second.delta != -450 {
		t.Fatalf("second: total=%d delta=%v", second.total, second.delta)
	}

	third := newReg(0, 16050, 5500)
	calculate(third, kinds, second)
	if third.total != 10550 || third.delta == nil || *third.delta != 6000 {
		t.Fatalf("third: total=%d delta=%v", third.total, third.delta)
	}
}
