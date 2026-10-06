package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/cache"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestResponseCache(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_URL é obrigatória no CI")
		}
		t.Skip("defina TEST_REDIS_URL para testar o cache")
	}
	gin.SetMode(gin.TestMode)
	db, _ := testutil.Database(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	for _, email := range []string{"cache-alice@example.com", "cache-bob@example.com"} {
		if _, err := users.Create(context.Background(), user.CreateInput{Name: "Pessoa", Email: email, Password: "Strong-pass-123!"}); err != nil {
			t.Fatal(err)
		}
	}
	server := api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	))
	responses, err := cache.New(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { responses.Close() })
	router := newRouter(server)
	registerBalanceModules(router, server, sqlx.NewDb(pool, "pgx"), responses.Middleware)

	request := func(router http.Handler, method, path, body, token string, want int) (string, string) {
		t.Helper()
		response := goalRequest(t, router, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.String(), response.Header().Get("X-Cache")
	}
	get := func(path, token string) (string, string) {
		t.Helper()
		return request(router, http.MethodGet, path, "", token, http.StatusOK)
	}
	login := func(email string) string {
		t.Helper()
		body, _ := request(router, http.MethodPost, "/sessions", fmt.Sprintf(`{"email":%q,"password":"Strong-pass-123!"}`, email), "", http.StatusOK)
		var session struct{ Token string }
		if err := json.Unmarshal([]byte(body), &session); err != nil {
			t.Fatal(err)
		}
		return "Bearer " + session.Token
	}
	alice := login("cache-alice@example.com")
	bob := login("cache-bob@example.com")

	// a segunda leitura igual vem do Redis, com o mesmo corpo
	first, state := get("/accounts", alice)
	if state != "MISS" {
		t.Fatalf("primeira leitura: X-Cache %q", state)
	}
	if again, state := get("/accounts", alice); state != "HIT" || again != first {
		t.Fatalf("segunda leitura: X-Cache %q, corpo %s (antes %s)", state, again, first)
	}

	// qualquer escrita invalida: a conta nova aparece na hora
	request(router, http.MethodPost, "/accounts", `{"name":"Inter","kind":"asset"}`, alice, http.StatusCreated)
	if body, state := get("/accounts", alice); state != "MISS" || !strings.Contains(body, "Inter") {
		t.Fatalf("depois de criar a conta: X-Cache %q, corpo %s", state, body)
	}

	// cada usuário tem o próprio cache
	if body, _ := get("/accounts", bob); body != "[]" {
		t.Fatalf("bob recebeu o cache da alice: %s", body)
	}

	// a exportação (dados completos) nunca é guardada
	get("/export", alice)
	if _, state := get("/export", alice); state == "HIT" {
		t.Fatal("exportação veio do cache")
	}

	// Redis fora do ar: a API responde do banco, sem cache e sem ficar esperando por ele
	down, err := cache.New("redis://127.0.0.1:1/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { down.Close() })
	broken := newRouter(server)
	registerBalanceModules(broken, server, sqlx.NewDb(pool, "pgx"), down.Middleware)
	start := time.Now()
	if body, state := request(broken, http.MethodGet, "/accounts", "", alice, http.StatusOK); state != "" || !strings.Contains(body, "Inter") {
		t.Fatalf("sem Redis: X-Cache %q, corpo %s", state, body)
	}
	request(broken, http.MethodPost, "/accounts", `{"name":"Fatura","kind":"liability"}`, alice, http.StatusCreated)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sem Redis, leitura e escrita levaram %v: o cache não pode segurar a API", elapsed)
	}
}
