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
	"github.com/kgazineu/finApp-back/internal/dataexport"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestExportImportRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := testutil.Database(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	for _, email := range []string{"export-alice@example.com", "export-bob@example.com"} {
		if _, err := users.Create(context.Background(), user.CreateInput{Name: "Pessoa", Email: email, Password: "Strong-pass-123!"}); err != nil {
			t.Fatal(err)
		}
	}
	server := api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	), api.WithGoalService(goal.NewService(postgres.NewGoalRepository(db))))
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
	alice := login("export-alice@example.com")
	bob := login("export-bob@example.com")

	// Alice usa um pouco de tudo, inclusive histórico que só existe arquivado
	var bank, card struct{ ID int64 }
	decode(call(http.MethodPost, "/accounts", `{"name":"Inter","kind":"asset","hasYield":true}`, alice, http.StatusCreated), &bank)
	decode(call(http.MethodPost, "/accounts", `{"name":"Fatura","kind":"liability"}`, alice, http.StatusCreated), &card)
	call(http.MethodPost, "/billings", fmt.Sprintf(`{"entries":[{"accountId":%d,"amount":10000},{"accountId":%d,"amount":3000}]}`, bank.ID, card.ID), alice, http.StatusCreated)
	call(http.MethodDelete, fmt.Sprintf("/accounts/%d", card.ID), "", alice, http.StatusOK) // arquivada: está no registro de saldo

	now := time.Now()
	call(http.MethodPost, "/recurring-transactions", fmt.Sprintf(
		`{"description":"Celular","kind":"expense","isFixed":false,"amount":500,"startMonth":%q,"installments":2,"dayOfMonth":1}`,
		now.Format("2006-01")), alice, http.StatusCreated)
	call(http.MethodPost, "/receivables", fmt.Sprintf(
		`{"kind":"loan","debtor":"João","description":"Bicicleta","amount":1000,"interestRate":5,"installments":3,"firstDueDate":%q}`,
		now.Format(time.DateOnly)), alice, http.StatusCreated)
	var projection struct {
		ProjectedAmount     int64
		PendingTransactions []struct{ ID int64 }
		PendingReceivables  []struct{ ID int64 }
	}
	decode(call(http.MethodGet, "/billings?months=3", "", alice, http.StatusOK), &projection)
	call(http.MethodPatch, fmt.Sprintf("/recurring-transactions/installments/%d", projection.PendingTransactions[0].ID), `{"paid":true}`, alice, http.StatusOK)
	call(http.MethodPatch, fmt.Sprintf("/receivables/installments/%d", projection.PendingReceivables[0].ID), `{"paid":true}`, alice, http.StatusOK)
	call(http.MethodPost, "/transactions", `{"kind":"expense","amountMinor":1299,"necessityLevel":3,"description":"Mercado","category":"Alimentação","paymentMethod":"pix","installments":1,"occurredAt":"2026-09-30T12:00:00Z"}`, alice, http.StatusCreated)
	call(http.MethodPut, "/savings-goal", `{"percent":30}`, alice, http.StatusOK) // vai junto no arquivo
	var reserve struct{ ID string }
	decode(call(http.MethodPost, "/goals", `{"name":"Reserva","targetMinor":100000}`, alice, http.StatusCreated), &reserve)
	call(http.MethodPost, "/goals/"+reserve.ID+"/allocations", `{"direction":"deposit","amountMinor":5000}`, alice, http.StatusOK)

	exported := call(http.MethodGet, "/export", "", alice, http.StatusOK)

	// arquivo inválido é recusado; a importação certa recria tudo
	call(http.MethodPost, "/import", `{"format":"outro","version":1}`, bob, http.StatusBadRequest)
	call(http.MethodPost, "/import", string(exported), bob, http.StatusOK)

	// conta com dados: sem replace é 409 (o front pede confirmação); arquivo com erro desfaz a troca inteira
	call(http.MethodPost, "/import", string(exported), bob, http.StatusConflict)
	before := normalize(t, call(http.MethodGet, "/export", "", bob, http.StatusOK))
	call(http.MethodPost, "/import?replace=true",
		`{"format":"finapp-export","version":1,"accounts":[{"id":1,"name":"X","kind":"outro","createdAt":"2026-01-01T00:00:00Z"}]}`, bob, http.StatusBadRequest)
	if after := normalize(t, call(http.MethodGet, "/export", "", bob, http.StatusOK)); after != before {
		t.Fatalf("importação com erro mexeu nos dados:\nantes:  %s\ndepois: %s", before, after)
	}
	// com replace, troca tudo pelo arquivo, sem duplicar (comparado logo abaixo)
	call(http.MethodPost, "/import?replace=true", string(exported), bob, http.StatusOK)

	if a, b := normalize(t, exported), normalize(t, call(http.MethodGet, "/export", "", bob, http.StatusOK)); a != b {
		t.Fatalf("exportação do Bob difere da da Alice:\nAlice: %s\nBob:   %s", a, b)
	}
	var bobProjection struct{ ProjectedAmount int64 }
	decode(call(http.MethodGet, "/billings?months=3", "", alice, http.StatusOK), &projection)
	decode(call(http.MethodGet, "/billings?months=3", "", bob, http.StatusOK), &bobProjection)
	if bobProjection.ProjectedAmount != projection.ProjectedAmount {
		t.Fatalf("projeção diferente depois de importar: Alice %d, Bob %d", projection.ProjectedAmount, bobProjection.ProjectedAmount)
	}
}

// normalize tira o que muda de um banco para outro (data da exportação e ids das contas).
func normalize(t *testing.T, data []byte) string {
	t.Helper()
	var doc dataexport.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc.ExportedAt = time.Time{}
	position := map[int64]int64{}
	for i := range doc.Accounts {
		position[doc.Accounts[i].ID] = int64(i)
		doc.Accounts[i].ID = 0
	}
	for i := range doc.BillingRegistrations {
		for j := range doc.BillingRegistrations[i].Entries {
			e := &doc.BillingRegistrations[i].Entries[j]
			e.AccountID = position[e.AccountID]
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
