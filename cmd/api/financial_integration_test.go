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
	if count != 2 {
		t.Fatalf("rejected requests must not create transactions; got %d", count)
	}
}
