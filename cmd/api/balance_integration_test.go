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
	var registration struct{ Total, Delta int64 }
	decode(call(http.MethodPost, "/billings", entries, alice, http.StatusCreated), &registration)
	if registration.Total != 7000 || registration.Delta != 0 {
		t.Fatalf("unexpected registration: %+v", registration)
	}

	// despesa de 500 no dia 1 deste mês e empréstimo de 1000 vencendo hoje entram na projeção
	now := time.Now()
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Celular","kind":"expense","isFixed":false,"amount":500,"startMonth":%q,"installments":2,"dayOfMonth":1}`,
		now.Format("2006-01")), alice, http.StatusCreated)
	call(http.MethodPost, "/receivables", fmt.Sprintf(
		`{"kind":"loan","debtor":"João","description":"Bicicleta","amount":1000,"firstDueDate":%q}`,
		now.Format(time.DateOnly)), alice, http.StatusCreated)

	type projection struct {
		ProjectedAmount     int64
		PendingTransactions []struct{ ID int64 }
		PendingReceivables  []struct{ ID int64 }
	}
	var p projection
	decode(call(http.MethodGet, "/billings?months=1", "", alice, http.StatusOK), &p)
	if p.ProjectedAmount != 7000-500+1000 || len(p.PendingTransactions) != 1 || len(p.PendingReceivables) != 1 {
		t.Fatalf("unexpected projection: %+v", p)
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
	if p.ProjectedAmount != 7000 {
		t.Fatalf("paid installments still projected: %+v", p)
	}

	// conta com histórico é arquivada, não apagada
	var deleted struct{ Message string }
	decode(call(http.MethodDelete, fmt.Sprintf("/accounts/%d", bank.ID), "", alice, http.StatusOK), &deleted)
	if deleted.Message != "account archived" {
		t.Fatalf("expected archive, got %q", deleted.Message)
	}
}
