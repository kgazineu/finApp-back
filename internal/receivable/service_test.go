package receivable

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func TestBuildInstallments(t *testing.T) {
	tests := []struct {
		name         string
		amount       int64
		mode         AmountMode
		interestRate int
		n            int
		firstDue     string
		wantAmounts  []int64
		wantDues     []string
	}{
		{"caso 1: pizza", 25, AmountTotal, 0, 1, "2026-10-31", []int64{25}, []string{"2026-10-31"}},
		{"caso 3: bicicleta", 1000, AmountTotal, 0, 2, "2026-10-31", []int64{500, 500}, []string{"2026-10-31", "2026-11-30"}},
		{"caso 4: bicicleta com 5%", 1000, AmountTotal, 5, 2, "2026-10-31", []int64{525, 525}, []string{"2026-10-31", "2026-11-30"}},
		{"resto e fim de mês", 1000, AmountTotal, 0, 3, "2026-10-31", []int64{333, 333, 334}, []string{"2026-10-31", "2026-11-30", "2026-12-31"}},
		{"valor por parcela, começando no passado", 5000, AmountInstallment, 0, 3, "2026-08-10", []int64{5000, 5000, 5000}, []string{"2026-08-10", "2026-09-10", "2026-10-10"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildInstallments(tt.amount, tt.mode, tt.interestRate, tt.n, date(tt.firstDue))
			if len(got) != len(tt.wantAmounts) {
				t.Fatalf("got %d installments, want %d", len(got), len(tt.wantAmounts))
			}
			for i, inst := range got {
				due := inst.DueDate.Format(time.DateOnly)
				if inst.Number != i+1 || inst.Amount != tt.wantAmounts[i] || due != tt.wantDues[i] {
					t.Errorf("parcela %d: got #%d %d %s, want #%d %d %s",
						i+1, inst.Number, inst.Amount, due, i+1, tt.wantAmounts[i], tt.wantDues[i])
				}
			}
		})
	}
}

func TestOverdue(t *testing.T) {
	inst := &Installment{DueDate: date("2026-10-31")}

	if inst.Overdue(time.Date(2026, 10, 31, 23, 0, 0, 0, time.Local)) {
		t.Error("no dia do vencimento ainda não está atrasada")
	}
	if !inst.Overdue(time.Date(2026, 11, 1, 0, 30, 0, 0, time.Local)) {
		t.Error("no dia seguinte está atrasada")
	}

	paidAt := time.Now()
	inst.PaidAt = &paidAt
	if inst.Overdue(time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)) {
		t.Error("parcela recebida nunca está atrasada")
	}
}
