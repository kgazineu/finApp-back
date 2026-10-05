package recurring

import (
	"fmt"
	"testing"
	"time"
)

func parseMonth(s string) time.Time {
	t, _ := time.Parse(monthLayout, s)
	return t
}

func TestOccurrenceIn(t *testing.T) {
	day := func(d int) *int { return &d }
	end := func(s string) *time.Time { m := parseMonth(s); return &m }

	salario := &Transaction{IsFixed: true, StartMonth: parseMonth("2026-10"), IntervalMonths: 1, DayOfMonth: day(5)}
	ipva := &Transaction{IsFixed: true, StartMonth: parseMonth("2027-01"), IntervalMonths: 12}
	conserto := &Transaction{StartMonth: parseMonth("2026-10"), IntervalMonths: 1, EndMonth: end("2026-10"), DayOfMonth: day(15)}
	celular := &Transaction{StartMonth: parseMonth("2026-11"), IntervalMonths: 1, EndMonth: end("2027-08")}
	aluguel := &Transaction{IsFixed: true, StartMonth: parseMonth("2026-10"), IntervalMonths: 1, DayOfMonth: day(31)}
	aluguelEncerrado := &Transaction{IsFixed: true, StartMonth: parseMonth("2026-10"), IntervalMonths: 1, EndMonth: end("2026-12")}

	tests := []struct {
		name             string
		tx               *Transaction
		month            string
		wantOK           bool
		wantDate         string
		wantInstallment  int
		wantInstallments int
	}{
		{"salário: antes do início", salario, "2026-09", false, "", 0, 0},
		{"salário: todo mês dia 5", salario, "2027-03", true, "2027-03-05", 0, 0},
		{"ipva: a cada 12 meses", ipva, "2028-01", true, "", 0, 0},
		{"ipva: fora do intervalo", ipva, "2027-06", false, "", 0, 0},
		{"conserto: uma vez só, sem 1/1", conserto, "2026-10", true, "2026-10-15", 0, 0},
		{"conserto: depois do fim", conserto, "2026-11", false, "", 0, 0},
		{"celular: primeira parcela", celular, "2026-11", true, "", 1, 10},
		{"celular: última parcela", celular, "2027-08", true, "", 10, 10},
		{"celular: depois da última", celular, "2027-09", false, "", 0, 0},
		{"dia 31 em fevereiro", aluguel, "2027-02", true, "2027-02-28", 0, 0},
		{"fixa encerrada não vira parcela", aluguelEncerrado, "2026-11", true, "", 0, 0},
		{"fixa encerrada: depois do fim", aluguelEncerrado, "2027-01", false, "", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, ok := tt.tx.OccurrenceIn(parseMonth(tt.month))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}

			date := ""
			if o.Date != nil {
				date = o.Date.Format(time.DateOnly)
			}

			if date != tt.wantDate || o.Installment != tt.wantInstallment || o.Installments != tt.wantInstallments {
				t.Errorf("got %q %d/%d, want %q %d/%d",
					date, o.Installment, o.Installments, tt.wantDate, tt.wantInstallment, tt.wantInstallments)
			}
		})
	}
}

func TestInstallmentsBetween(t *testing.T) {
	day := func(d int) *int { return &d }
	end := func(s string) *time.Time { m := parseMonth(s); return &m }
	date := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }

	tests := []struct {
		name  string
		tx    *Transaction
		from  string
		until string
		want  []string
	}{
		{"salário todo dia 5", &Transaction{StartMonth: parseMonth("2026-10"), IntervalMonths: 1, DayOfMonth: day(5)},
			"2026-10-01", "2026-12-31", []string{"#1 2026-10-05", "#2 2026-11-05", "#3 2026-12-05"}},
		{"sem dia certo vence no último dia", &Transaction{StartMonth: parseMonth("2026-10"), IntervalMonths: 1},
			"2026-10-01", "2026-11-30", []string{"#1 2026-10-31", "#2 2026-11-30"}},
		{"a cada 12 meses", &Transaction{StartMonth: parseMonth("2027-01"), IntervalMonths: 12},
			"2027-01-01", "2028-01-31", []string{"#1 2027-01-31", "#2 2028-01-31"}},
		{"cadastrada depois: pula os meses antigos e mantém a numeração", &Transaction{StartMonth: parseMonth("2026-08"), IntervalMonths: 1, EndMonth: end("2027-05")},
			"2026-10-01", "2026-10-31", []string{"#3 2026-10-31"}},
		{"encerrada não gera depois do fim", &Transaction{StartMonth: parseMonth("2026-10"), IntervalMonths: 1, EndMonth: end("2026-11")},
			"2026-10-01", "2027-01-31", []string{"#1 2026-10-31", "#2 2026-11-30"}},
		{"dia 31 em meses curtos", &Transaction{StartMonth: parseMonth("2026-11"), IntervalMonths: 1, DayOfMonth: day(31)},
			"2026-11-01", "2027-02-28", []string{"#1 2026-11-30", "#2 2026-12-31", "#3 2027-01-31", "#4 2027-02-28"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, i := range tt.tx.InstallmentsBetween(date(tt.from), date(tt.until)) {
				got = append(got, fmt.Sprintf("#%d %s", i.Number, i.DueDate.Format(time.DateOnly)))
			}

			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndMonthFor(t *testing.T) {
	tests := []struct {
		name         string
		isFixed      bool
		interval     int
		installments int
		want         string
	}{
		{"variável sem quantas vezes: uma vez só", false, 1, 0, "2026-10"},
		{"fixa sem quantas vezes: sem fim", true, 1, 0, ""},
		{"variável em 10x", false, 1, 10, "2027-07"},
		{"fixa 3 vezes a cada 12 meses", true, 12, 3, "2028-10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &Transaction{IsFixed: tt.isFixed, StartMonth: parseMonth("2026-10"), IntervalMonths: tt.interval}

			got := ""
			if end := endMonthFor(tx, tt.installments); end != nil {
				got = end.Format(monthLayout)
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
