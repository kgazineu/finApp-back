package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestFinancialFlowEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	first, err := users.Create(context.Background(), user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "Strong-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := users.Create(context.Background(), user.CreateInput{Name: "Bob", Email: "bob@example.com", Password: "Another-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newRouter(api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	))))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}

	post := func(path, body, token string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data
	}
	assertStatus := func(want, got int, body []byte) {
		t.Helper()
		if got != want {
			t.Fatalf("expected HTTP %d, got %d: %s", want, got, body)
		}
	}

	loginBody := `{"email":"alice@example.com","password":"Strong-pass-123!"}`
	status, body := post("/sessions", `{"email":"alice@example.com","password":"wrong"}`, "")
	assertStatus(http.StatusUnauthorized, status, body)
	status, body = post("/sessions", loginBody, "")
	assertStatus(http.StatusOK, status, body)
	var session struct {
		Token     string    `json:"token"`
		TokenType string    `json:"tokenType"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.TokenType != "Bearer" || !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("invalid session response: %+v", session)
	}
	secret, err := base64.RawURLEncoding.DecodeString(session.Token)
	if err != nil || len(secret) != 32 {
		t.Fatalf("expected 32-byte opaque token: %v", err)
	}
	wantHash := sha256.Sum256(secret)
	var storedHash []byte
	var sessionOwner uuid.UUID
	if err := db.Raw("SELECT token_hash, user_id FROM sessions").Row().Scan(&storedHash, &sessionOwner); err != nil {
		t.Fatal(err)
	}
	if sessionOwner != first.ID || !bytes.Equal(storedHash, wantHash[:]) || bytes.Equal(storedHash, secret) {
		t.Fatal("session must store the first user's ID and only the token hash")
	}
	var rawTokenRows int64
	if err := db.Raw("SELECT count(*) FROM sessions WHERE encode(token_hash, 'base64') = ? OR encode(token_hash, 'hex') = ?", session.Token, session.Token).Row().Scan(&rawTokenRows); err != nil {
		t.Fatal(err)
	}
	if rawTokenRows != 0 {
		t.Fatal("raw bearer token found in sessions")
	}

	const txBody = `{"amountMinor":987654321,"necessityLevel":4}`
	status, body = post("/transactions", txBody, "")
	assertStatus(http.StatusUnauthorized, status, body)
	status, body = post("/transactions", txBody, "not-a-valid-token")
	assertStatus(http.StatusUnauthorized, status, body)
	status, body = post("/transactions", txBody, session.Token)
	assertStatus(http.StatusCreated, status, body)
	var created struct {
		ID             uuid.UUID `json:"id"`
		AmountMinor    int64     `json:"amountMinor"`
		NecessityLevel int       `json:"necessityLevel"`
		CreatedAt      time.Time `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil || created.AmountMinor != 987654321 || created.NecessityLevel != 4 || created.CreatedAt.IsZero() {
		t.Fatalf("unexpected transaction response: %+v", created)
	}
	var owner uuid.UUID
	var amount int64
	var level int
	if err := db.Raw("SELECT user_id, amount_minor, necessity_level FROM transactions WHERE id = ?", created.ID).Row().Scan(&owner, &amount, &level); err != nil {
		t.Fatal(err)
	}
	if owner != first.ID || owner == second.ID || amount != created.AmountMinor || level != created.NecessityLevel {
		t.Fatalf("wrong persisted transaction: owner=%s amount=%d level=%d", owner, amount, level)
	}

	status, body = post("/sessions", `{"email":"bob@example.com","password":"Another-pass-123!"}`, "")
	assertStatus(http.StatusOK, status, body)
	var otherSession struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &otherSession); err != nil || otherSession.Token == "" || otherSession.Token == session.Token {
		t.Fatalf("expected independent second session: %s (%v)", body, err)
	}
	status, body = post("/transactions", `{"amountMinor":42,"necessityLevel":1}`, otherSession.Token)
	assertStatus(http.StatusCreated, status, body)
	var otherTx struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(body, &otherTx); err != nil || otherTx.ID == uuid.Nil {
		t.Fatalf("invalid second transaction: %s (%v)", body, err)
	}
	if err := db.Raw("SELECT user_id, amount_minor, necessity_level FROM transactions WHERE id = ?", otherTx.ID).Row().Scan(&owner, &amount, &level); err != nil {
		t.Fatal(err)
	}
	if owner != second.ID || owner == first.ID || amount != 42 || level != 1 {
		t.Fatalf("second user's transaction assigned incorrectly: owner=%s amount=%d level=%d", owner, amount, level)
	}

	for _, tc := range []struct {
		token  string
		wantID uuid.UUID
	}{
		{session.Token, created.ID}, {otherSession.Token, otherTx.ID},
	} {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/transactions?limit=1", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Data []struct {
				ID uuid.UUID `json:"id"`
			} `json:"data"`
			Limit int `json:"limit"`
		}
		err = json.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || page.Limit != 1 || len(page.Data) != 1 || page.Data[0].ID != tc.wantID {
			t.Fatalf("listagem deve conter apenas transação do titular: %+v status=%d err=%v", page, response.StatusCode, err)
		}
	}

	// Exercise the enriched HTTP contract through the real service, migration and repository.
	incomeDate := "2026-09-30T15:30:00Z"
	ownerIDs := map[uuid.UUID]map[uuid.UUID]bool{
		first.ID:  {created.ID: true},
		second.ID: {otherTx.ID: true},
	}
	for _, tc := range []struct {
		token, payload, kind, payment string
		owner                         uuid.UUID
		amount                        int64
		level                         int
	}{
		{session.Token, `{"amountMinor":75000,"kind":"income","description":"Salário","category":"Trabalho","installments":1,"occurredAt":"` + incomeDate + `"}`, "income", "", first.ID, 75000, 0},
		{session.Token, `{"amountMinor":23599,"kind":"expense","description":"Notebook","category":"Tecnologia","paymentMethod":"card","installments":12,"necessityLevel":2,"occurredAt":"` + incomeDate + `"}`, "expense", "card", first.ID, 23599, 2},
		{otherSession.Token, `{"amountMinor":100,"kind":"expense","description":"Café","category":"Alimentação","paymentMethod":"pix","installments":1,"necessityLevel":1,"occurredAt":"` + incomeDate + `"}`, "expense", "pix", second.ID, 100, 1},
	} {
		status, body = post("/transactions", tc.payload, tc.token)
		assertStatus(http.StatusCreated, status, body)
		var response map[string]any
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		idText, ok := response["id"].(string)
		if !ok {
			t.Fatalf("missing transaction ID: %s", body)
		}
		id, err := uuid.Parse(idText)
		if err != nil || id == uuid.Nil || response["kind"] != tc.kind || response["amountMinor"] != float64(tc.amount) || response["occurredAt"] != incomeDate {
			t.Fatalf("unexpected enriched response: %s (%v)", body, err)
		}
		if response["description"] == "" || response["category"] == "" || response["installments"] == nil {
			t.Fatalf("enriched fields missing: %s", body)
		}
		if tc.kind == "income" {
			if _, ok := response["necessityLevel"]; ok {
				t.Fatalf("income exposed necessityLevel: %s", body)
			}
			if _, ok := response["paymentMethod"]; ok {
				t.Fatalf("income exposed paymentMethod: %s", body)
			}
		} else if response["necessityLevel"] != float64(tc.level) || response["paymentMethod"] != tc.payment {
			t.Fatalf("expense fields missing: %s", body)
		}
		var persisted struct {
			Owner        uuid.UUID
			Kind         string
			Description  string
			Category     string
			Payment      *string
			Installments int
			Level        *int
			OccurredAt   time.Time
		}
		if err := db.Raw("SELECT user_id, kind, description, category, payment_method, installments, necessity_level, occurred_at FROM transactions WHERE id = ?", id).Row().Scan(&persisted.Owner, &persisted.Kind, &persisted.Description, &persisted.Category, &persisted.Payment, &persisted.Installments, &persisted.Level, &persisted.OccurredAt); err != nil {
			t.Fatal(err)
		}
		if persisted.Owner != tc.owner || persisted.Kind != tc.kind || persisted.Description != response["description"] || persisted.Category != response["category"] || persisted.Installments != int(response["installments"].(float64)) || persisted.OccurredAt.Format(time.RFC3339) != incomeDate {
			t.Fatalf("incorrect persisted transaction: %+v / %s", persisted, body)
		}
		ownerIDs[tc.owner][id] = true
		if tc.kind == "income" && (persisted.Level != nil || persisted.Payment != nil) {
			t.Fatalf("income expense columns should be NULL: %+v", persisted)
		}
		if tc.kind == "expense" && (persisted.Level == nil || *persisted.Level != tc.level || persisted.Payment == nil || *persisted.Payment != tc.payment) {
			t.Fatalf("expense columns not persisted: %+v", persisted)
		}
	}

	for _, tc := range []struct {
		token string
		owner uuid.UUID
		want  map[string]bool
	}{
		{session.Token, first.ID, map[string]bool{"income": true, "expense": true}},
		{otherSession.Token, second.ID, map[string]bool{"expense": true}},
	} {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/transactions?limit=10", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Data []map[string]any `json:"data"`
		}
		err = json.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(page.Data) != len(ownerIDs[tc.owner]) {
			t.Fatalf("unexpected owner page: %+v status=%d err=%v", page, response.StatusCode, err)
		}
		seen := make(map[string]bool)
		for _, item := range page.Data {
			idText, ok := item["id"].(string)
			if !ok {
				t.Fatalf("listed transaction missing ID: %+v", item)
			}
			id, err := uuid.Parse(idText)
			if err != nil || !ownerIDs[tc.owner][id] {
				t.Fatalf("transaction from another owner or unexpected ID: %+v", item)
			}
			if _, ok := item["userId"]; ok {
				t.Fatalf("owner leaked: %+v", item)
			}
			if item["occurredAt"] == incomeDate {
				seen[item["kind"].(string)] = true
				if item["kind"] == "income" {
					if _, ok := item["necessityLevel"]; ok {
						t.Fatalf("listed income exposed necessityLevel: %+v", item)
					}
					if _, ok := item["paymentMethod"]; ok {
						t.Fatalf("listed income exposed paymentMethod: %+v", item)
					}
				} else if item["paymentMethod"] == nil || item["necessityLevel"] == nil {
					t.Fatalf("listed expense lost details: %+v", item)
				}
			}
		}
		if len(seen) != len(tc.want) {
			t.Fatalf("missing detailed transactions: got %v want %v", seen, tc.want)
		}
	}
	for _, invalid := range []string{
		`{"amountMinor":10,"kind":"income","description":"Salário","category":"Trabalho","installments":1,"necessityLevel":2,"occurredAt":"2026-09-30T15:30:00Z"}`,
		`{"amountMinor":10,"kind":"expense","description":"Compra","category":"Casa","paymentMethod":"pix","installments":2,"necessityLevel":3,"occurredAt":"2026-09-30T15:30:00Z"}`,
	} {
		status, body = post("/transactions", invalid, session.Token)
		assertStatus(http.StatusBadRequest, status, body)
	}

	getStatus := func(token string) int {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+"/transactions", nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if got := getStatus(""); got != http.StatusUnauthorized {
		t.Fatalf("listagem anônima retornou %d", got)
	}
	if err := db.Exec("UPDATE sessions SET expires_at = ? WHERE token_hash = ?", time.Now().Add(-time.Minute), wantHash[:]).Error; err != nil {
		t.Fatal(err)
	}
	if got := getStatus(session.Token); got != http.StatusUnauthorized {
		t.Fatalf("listagem com sessão expirada retornou %d", got)
	}
	status, body = post("/transactions", txBody, session.Token)
	assertStatus(http.StatusUnauthorized, status, body)
	if bytes.Contains(body, []byte(session.Token)) {
		t.Fatal("expired token leaked in error response")
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM transactions").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("rejected requests must not create transactions; got %d", count)
	}
}
