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

func TestReceivableFixedInstallmentsPastStartAndEditing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := testutil.Database(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	for _, email := range []string{"rec-alice@example.com", "rec-bob@example.com"} {
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
	login := func(email string) string {
		t.Helper()
		var session struct{ Token string }
		if err := json.Unmarshal(call(http.MethodPost, "/sessions", fmt.Sprintf(`{"email":%q,"password":"Strong-pass-123!"}`, email), "", http.StatusOK), &session); err != nil {
			t.Fatal(err)
		}
		return "Bearer " + session.Token
	}
	alice := login("rec-alice@example.com")
	bob := login("rec-bob@example.com")

	type installment struct {
		ID      int64
		Number  int
		Amount  int64
		DueDate string
		PaidAt  *time.Time
		Overdue bool
	}
	type receivable struct {
		ID           int64
		AmountMode   string
		Installments []installment
	}
	list := func() receivable {
		t.Helper()
		var all []receivable
		if err := json.Unmarshal(call(http.MethodGet, "/receivables", "", alice, http.StatusOK), &all); err != nil || len(all) != 1 {
			t.Fatalf("listagem: %v %v", all, err)
		}
		return all[0]
	}
	amounts := func(r receivable) (out []int64) {
		for _, i := range r.Installments {
			out = append(out, i.Amount)
		}
		return out
	}
	dues := func(r receivable) (out []string) {
		for _, i := range r.Installments {
			out = append(out, i.DueDate)
		}
		return out
	}

	// assinatura do pai: R$ 50,00 por mês, 5 meses, começando dois meses atrás
	now := time.Now()
	start := time.Date(now.Year(), now.Month()-2, 10, 0, 0, 0, 0, time.UTC)
	month := func(i int) string { return start.AddDate(0, i, 0).Format(time.DateOnly) }
	body := fmt.Sprintf(`{"kind":"loan","debtor":"Pai","description":"Assinatura","amount":5000,"amountMode":"installment","installments":5,"firstDueDate":%q}`, month(0))

	call(http.MethodPost, "/receivables", `{"kind":"loan","debtor":"Pai","description":"x","amount":5000,"amountMode":"installment","interestRate":5,"installments":2,"firstDueDate":"2026-10-10"}`, alice, http.StatusBadRequest)
	call(http.MethodPost, "/receivables", `{"kind":"loan","debtor":"Pai","description":"x","amount":5000,"amountMode":"dobro","firstDueDate":"2026-10-10"}`, alice, http.StatusBadRequest)
	call(http.MethodPost, "/receivables", body, alice, http.StatusCreated)

	rec := list()
	if rec.AmountMode != "installment" || fmt.Sprint(amounts(rec)) != "[5000 5000 5000 5000 5000]" {
		t.Fatalf("parcelas fixas: modo %q, valores %v", rec.AmountMode, amounts(rec))
	}
	if !rec.Installments[0].Overdue || !rec.Installments[1].Overdue || rec.Installments[3].Overdue {
		t.Fatalf("começando no passado, as duas primeiras devem estar atrasadas: %+v", rec.Installments)
	}

	// ela pagou os dois primeiros meses; marcar é com a própria pessoa
	for _, i := range rec.Installments[:2] {
		call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", i.ID), `{"paid":true}`, alice, http.StatusOK)
	}

	// a assinatura aumentou a partir da 3ª parcela: as recebidas não mudam
	third := rec.Installments[2].ID
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", third), `{}`, alice, http.StatusBadRequest)
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", third), `{"amount":6000}`, bob, http.StatusNotFound)
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", third), `{"amount":6000,"applyToFollowing":true}`, alice, http.StatusOK)
	rec = list()
	if fmt.Sprint(amounts(rec)) != "[5000 5000 6000 6000 6000]" {
		t.Fatalf("aumento deveria valer da 3ª em diante: %v", amounts(rec))
	}

	// o vencimento passou para o dia 20 a partir da 4ª parcela
	fourth := rec.Installments[3]
	newDue := start.AddDate(0, 3, 10).Format(time.DateOnly)
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", fourth.ID), fmt.Sprintf(`{"dueDate":%q,"applyToFollowing":true}`, newDue), alice, http.StatusOK)
	rec = list()
	wantDues := fmt.Sprint([]string{month(0), month(1), month(2), newDue, start.AddDate(0, 4, 10).Format(time.DateOnly)})
	if fmt.Sprint(dues(rec)) != wantDues {
		t.Fatalf("vencimentos: %v, esperado %v", dues(rec), wantDues)
	}

	// só uma parcela, sem levar para as seguintes; e marcar como recebida junto
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", rec.Installments[4].ID), `{"amount":6500,"paid":true}`, alice, http.StatusOK)
	rec = list()
	if fmt.Sprint(amounts(rec)) != "[5000 5000 6000 6000 6500]" || rec.Installments[4].PaidAt == nil || rec.Installments[2].PaidAt != nil {
		t.Fatalf("edição de uma parcela só: %+v", rec.Installments)
	}
}
