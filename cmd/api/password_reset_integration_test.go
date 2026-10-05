package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/passwordreset"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

type mailbox chan string

func (m mailbox) Send(_ context.Context, _, _, body string) error {
	m <- body
	return nil
}

func TestPasswordResetEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := testutil.Database(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	if _, err := users.Create(context.Background(), user.CreateInput{Name: "Ana", Email: "reset@example.com", Password: "Old-pass-123!"}); err != nil {
		t.Fatal(err)
	}
	server := api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	))
	router := newRouter(server)
	sqlDB := sqlx.NewDb(pool, "pgx")
	registerBalanceModules(router, server, sqlDB)
	mail := make(mailbox, 1)
	passwordreset.NewModule(sqlDB, mail, password.Hasher{}).RegisterRoutes(router.Group("/password-resets"))

	call := func(path, body, token string, want int) []byte {
		t.Helper()
		response := goalRequest(t, router, http.MethodPost, path, body, token)
		if response.Code != want {
			t.Fatalf("POST %s: HTTP %d, want %d: %s", path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	var session struct{ Token string }
	if err := json.Unmarshal(call("/sessions", `{"email":"reset@example.com","password":"Old-pass-123!"}`, "", http.StatusOK), &session); err != nil {
		t.Fatal(err)
	}

	// e-mail desconhecido responde igual e não manda nada
	call("/password-resets", `{"email":"ninguem@example.com"}`, "", http.StatusAccepted)
	call("/password-resets", `{"email":"reset@example.com"}`, "", http.StatusAccepted)
	var code string
	select {
	case body := <-mail:
		code = regexp.MustCompile(`\d{6}`).FindString(body)
	case <-time.After(5 * time.Second):
		t.Fatal("código não enviado")
	}
	select {
	case <-mail:
		t.Fatal("e-mail desconhecido recebeu código")
	default:
	}

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	call("/password-resets/verify", `{"email":"reset@example.com","code":"`+wrong+`"}`, "", http.StatusBadRequest)
	call("/password-resets/verify", `{"email":"reset@example.com","code":"`+code+`"}`, "", http.StatusOK)
	call("/password-resets/confirm", `{"email":"reset@example.com","code":"`+code+`","password":"fraca"}`, "", http.StatusBadRequest)
	call("/password-resets/confirm", `{"email":"reset@example.com","code":"`+code+`","password":"New-pass-123!"}`, "", http.StatusOK)

	// código usado não vale de novo, sessões antigas caem e só a senha nova entra
	call("/password-resets/verify", `{"email":"reset@example.com","code":"`+code+`"}`, "", http.StatusBadRequest)
	if response := goalRequest(t, router, http.MethodGet, "/accounts", "", "Bearer "+session.Token); response.Code != http.StatusUnauthorized {
		t.Fatalf("old session still valid: HTTP %d", response.Code)
	}
	call("/sessions", `{"email":"reset@example.com","password":"Old-pass-123!"}`, "", http.StatusUnauthorized)
	call("/sessions", `{"email":"reset@example.com","password":"New-pass-123!"}`, "", http.StatusOK)
}
