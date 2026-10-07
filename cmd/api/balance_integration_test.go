package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestBalanceModulesEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := testutil.Database(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	for _, email := range []string{"balance-alice@example.com", "balance-bob@example.com"} {
		if _, err := users.Create(context.Background(), user.CreateInput{Name: "Pessoa", Email: email, Password: "Strong-pass-123!"}); err != nil {
			t.Fatal(err)
		}
	}
	server := api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	))
	router := newRouter(server)
	registerBalanceModules(router, server, sqlx.NewDb(pool, "pgx"))

	call := func(method, path, body, token string, want int) []byte {
		t.Helper()
		response := goalRequest(t, router, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	decode := func(data []byte, target any) {
		t.Helper()
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("%v: %s", err, data)
		}
	}
	login := func(email string) string {
		t.Helper()
		var session struct{ Token string }
		decode(call(http.MethodPost, "/sessions", fmt.Sprintf(`{"email":%q,"password":"Strong-pass-123!"}`, email), "", http.StatusOK), &session)
		return "Bearer " + session.Token
	}
	alice := login("balance-alice@example.com")
	bob := login("balance-bob@example.com")

	// sem sessão válida nenhuma rota responde
	for _, path := range []string{"/accounts", "/billings", "/recurring-transactions", "/receivables"} {
		call(http.MethodGet, path, "", "", http.StatusUnauthorized)
		call(http.MethodGet, path, "", "Bearer invalid", http.StatusUnauthorized)
	}

	var bank, card struct{ ID int64 }
	decode(call(http.MethodPost, "/accounts", `{"name":"Inter","kind":"asset","hasYield":true}`, alice, http.StatusCreated), &bank)
	decode(call(http.MethodPost, "/accounts", `{"name":"Fatura","kind":"liability"}`, alice, http.StatusCreated), &card)
	call(http.MethodPatch, fmt.Sprintf("/accounts/%d", bank.ID), `{"name":"Inter","kind":"liability","hasYield":true}`, alice, http.StatusBadRequest)

	// Bob não enxerga nem altera nada da Alice
	if body := call(http.MethodGet, "/accounts", "", bob, http.StatusOK); string(body) != "[]" {
		t.Fatalf("bob sees alice's accounts: %s", body)
	}
	call(http.MethodGet, fmt.Sprintf("/accounts/%d", bank.ID), "", bob, http.StatusNotFound)
	call(http.MethodPatch, fmt.Sprintf("/accounts/%d", bank.ID), `{"name":"x","kind":"asset"}`, bob, http.StatusNotFound)
	call(http.MethodDelete, fmt.Sprintf("/accounts/%d", bank.ID), "", bob, http.StatusNotFound)
	call(http.MethodPost, "/billings", fmt.Sprintf(`{"entries":[{"accountId":%d,"amount":1}]}`, bank.ID), bob, http.StatusBadRequest)

	// saldo: 10000 no banco − 3000 na fatura = 7000
	entries := fmt.Sprintf(`{"entries":[{"accountId":%d,"amount":10000},{"accountId":%d,"amount":3000}]}`, bank.ID, card.ID)
	var registration struct {
		Total   int64
		Delta   *int64
		Entries []struct{ AccountKind string }
	}
	decode(call(http.MethodPost, "/billings", entries, alice, http.StatusCreated), &registration)
	if registration.Total != 7000 || registration.Delta != nil {
		t.Fatalf("primeiro registro: total %d, delta %v (sem registro anterior não há delta)", registration.Total, registration.Delta)
	}
	if kinds := fmt.Sprint(registration.Entries); kinds != "[{asset} {liability}]" {
		t.Fatalf("tipo das contas nos lançamentos (separa saldo de fatura): %s", kinds)
	}
	decode(call(http.MethodPost, "/billings", entries, alice, http.StatusCreated), &registration)
	if registration.Delta == nil || *registration.Delta != 0 {
		t.Fatalf("segundo registro com o mesmo saldo deveria ter delta 0, veio %v", registration.Delta)
	}

	// a projeção de 1 mês é para o dia 1 do mês que vem e conta o mês que vem inteiro: entram as duas
	// parcelas de 500 do celular (dia 1 deste mês e do próximo) e o empréstimo de 1000 vencendo hoje
	now := time.Now()
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Celular","kind":"expense","isFixed":false,"amount":500,"startMonth":%q,"installments":2,"dayOfMonth":1}`,
		now.Format("2006-01")), alice, http.StatusCreated)
	call(http.MethodPost, "/receivables", fmt.Sprintf(
		`{"kind":"loan","debtor":"João","description":"Bicicleta","amount":1000,"firstDueDate":%q}`,
		now.Format(time.DateOnly)), alice, http.StatusCreated)

	type projection struct {
		ProjectedAmount      int64
		PendingTransactions  []struct{ ID int64 }
		PendingReceivables   []struct{ ID int64 }
		BillingRegistrations []struct {
			Entries []struct{ AccountKind string }
		}
	}
	var p projection
	decode(call(http.MethodGet, "/billings?months=1", "", alice, http.StatusOK), &p)
	if p.ProjectedAmount != 7000-2*500+1000 || len(p.PendingTransactions) != 2 || len(p.PendingReceivables) != 1 {
		t.Fatalf("unexpected projection: %+v", p)
	}
	if kinds := fmt.Sprint(p.BillingRegistrations[0].Entries); kinds != "[{asset} {liability}]" {
		t.Fatalf("tipo das contas nos registros da projeção: %s", kinds)
	}
	var empty projection
	decode(call(http.MethodGet, "/billings?months=1", "", bob, http.StatusOK), &empty)
	if empty.ProjectedAmount != 0 || len(empty.PendingTransactions) != 0 || len(empty.PendingReceivables) != 0 {
		t.Fatalf("bob sees alice's projection: %+v", empty)
	}

	// Bob não marca parcelas da Alice; a Alice marca e elas saem da projeção
	installment := fmt.Sprintf("/recurring-transactions/installments/%d", p.PendingTransactions[0].ID)
	received := fmt.Sprintf("/receivables/installments/%d", p.PendingReceivables[0].ID)
	call(http.MethodPatch, installment, `{"paid":true}`, bob, http.StatusNotFound)
	call(http.MethodPatch, received, `{"paid":true}`, bob, http.StatusNotFound)
	call(http.MethodPatch, installment, `{"paid":true}`, alice, http.StatusOK)
	call(http.MethodPatch, received, `{"paid":true}`, alice, http.StatusOK)
	decode(call(http.MethodGet, "/billings?months=1", "", alice, http.StatusOK), &p)
	if p.ProjectedAmount != 7000-500 {
		t.Fatalf("paid installments still projected: %+v", p)
	}

	// conta com histórico é arquivada, não apagada
	var deleted struct{ Message string }
	decode(call(http.MethodDelete, fmt.Sprintf("/accounts/%d", bank.ID), "", alice, http.StatusOK), &deleted)
	if deleted.Message != "conta arquivada" {
		t.Fatalf("expected archive, got %q", deleted.Message)
	}

	// começou dois meses antes do cadastro: as parcelas desde o início aparecem, atrasadas, para marcar as já pagas
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Seguro","kind":"expense","isFixed":false,"amount":100,"startMonth":%q,"installments":12,"dayOfMonth":1}`,
		time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")), bob, http.StatusCreated)
	var past struct {
		PendingTransactions []struct {
			Number  int
			Overdue bool
		}
	}
	decode(call(http.MethodGet, "/billings?months=1", "", bob, http.StatusOK), &past)
	if pt := past.PendingTransactions; len(pt) != 4 || pt[0].Number != 1 || !pt[0].Overdue || !pt[1].Overdue {
		t.Fatalf("parcelas desde o início, atrasadas: %+v", pt)
	}

	// crescimento por mês: o mês que vem pelas regras, pago ou não. Entradas + a receber − despesas
	// (fixas e variáveis: o seguro de 100 entra, como na projeção)
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	thisMonth := now.Format("2006-01")
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Salário","kind":"income","isFixed":true,"amount":5000,"startMonth":%q,"dayOfMonth":5}`, thisMonth), bob, http.StatusCreated)
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Streaming","kind":"expense","isFixed":true,"amount":110,"startMonth":%q}`, thisMonth), bob, http.StatusCreated)
	call(http.MethodPost, "/receivables", fmt.Sprintf(
		`{"kind":"loan","debtor":"Pai","description":"Assinatura","amount":50,"amountMode":"installment","installments":2,"firstDueDate":%q}`,
		nextMonth.AddDate(0, 0, 9).Format(time.DateOnly)), bob, http.StatusCreated)
	var growth struct {
		MonthlyGrowth       int64
		MonthlyReceivables  int64
		MonthlyGrowthMonth  string
		PendingTransactions []struct {
			ID          int64
			Description string
			DueDate     string
		}
	}
	decode(call(http.MethodGet, "/billings?months=1", "", bob, http.StatusOK), &growth)
	if growth.MonthlyGrowth != 5000-110-100+50 || growth.MonthlyReceivables != 50 || growth.MonthlyGrowthMonth != nextMonth.Format("2006-01") {
		t.Fatalf("crescimento por mês: %d (recebimento %d) em %s", growth.MonthlyGrowth, growth.MonthlyReceivables, growth.MonthlyGrowthMonth)
	}
	// receber adiantado o salário do mês que vem não muda o crescimento
	marked := false
	for _, i := range growth.PendingTransactions {
		if i.Description == "Salário" && i.DueDate == nextMonth.AddDate(0, 0, 4).Format(time.DateOnly) {
			call(http.MethodPatch, fmt.Sprintf("/recurring-transactions/installments/%d", i.ID), `{"paid":true}`, bob, http.StatusOK)
			marked = true
		}
	}
	if !marked {
		t.Fatalf("salário do mês que vem não está nas pendentes: %+v", growth.PendingTransactions)
	}
	decode(call(http.MethodGet, "/billings?months=1", "", bob, http.StatusOK), &growth)
	if growth.MonthlyGrowth != 5000-110-100+50 {
		t.Fatalf("marcar adiantado mudou o crescimento: %d", growth.MonthlyGrowth)
	}

	// meta de guardar: um dos dois (porcentagem 1..100 ou valor > 0), cada usuário com a sua; vazio remove
	call(http.MethodPut, "/savings-goal", `{"percent":50,"amount":1000}`, bob, http.StatusBadRequest)
	call(http.MethodPut, "/savings-goal", `{"percent":0}`, bob, http.StatusBadRequest)
	call(http.MethodPut, "/savings-goal", `{"percent":101}`, bob, http.StatusBadRequest)
	call(http.MethodPut, "/savings-goal", `{"percent":50}`, bob, http.StatusOK)
	call(http.MethodPut, "/savings-goal", `{"amount":150000}`, alice, http.StatusOK)
	for token, want := range map[string]string{bob: `{"percent":50,"amount":null}`, alice: `{"percent":null,"amount":150000}`} {
		if got := string(call(http.MethodGet, "/savings-goal", "", token, http.StatusOK)); got != want {
			t.Fatalf("meta: %s, esperado %s", got, want)
		}
	}
	call(http.MethodPut, "/savings-goal", `{}`, bob, http.StatusOK)
	if got := string(call(http.MethodGet, "/savings-goal", "", bob, http.StatusOK)); got != `{"percent":null,"amount":null}` {
		t.Fatalf("meta removida: %s", got)
	}
}
