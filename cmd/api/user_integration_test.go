package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestUserRegistrationEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	service := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	server := httptest.NewServer(newRouter(api.NewServer(service, api.WithFinancialServices(auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}), nil))))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	payload := `{"name":"Kaian","email":"kaian@example.com","password":"Uma-senha-de-teste-123!"}`
	response, err := client.Post(server.URL+"/users", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("esperado 201, recebido %d", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 {
		t.Fatal("a resposta deve conter apenas os cinco campos públicos")
	}
	for _, key := range []string{"id", "name", "email", "createdAt", "updatedAt"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("campo público ausente: %s", key)
		}
	}
	var hash string
	if err := db.Raw("SELECT password_hash FROM users WHERE id = ?", body["id"]).Row().Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == "Uma-senha-de-teste-123!" {
		t.Fatal("senha persistida em texto puro")
	}
	match, err := (password.Hasher{}).Verify("Uma-senha-de-teste-123!", hash)
	if err != nil || !match {
		t.Fatal("hash persistido não verifica a senha original")
	}
	for _, tt := range []struct {
		body   string
		status int
	}{
		{payload, 409},
		{`{"name":"Kaian","email":"outro@example.com","password":"curta"}`, 400},
		{`{"name":"   ","email":"outro@example.com","password":"Uma-senha-de-teste-123!"}`, 400},
	} {
		response, err := client.Post(server.URL+"/users", "application/json", strings.NewReader(tt.body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tt.status {
			t.Errorf("esperado %d, recebido %d", tt.status, response.StatusCode)
		}
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM users").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("esperado apenas um cadastro, recebido %d", count)
	}

	login, err := client.Post(server.URL+"/sessions", "application/json", strings.NewReader(`{"email":"kaian@example.com","password":"Uma-senha-de-teste-123!"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusOK || session.Token == "" {
		t.Fatalf("login failed: %d", login.StatusCode)
	}
	for _, offset := range []string{"0", "1"} {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/users?limit=1&offset="+offset, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+session.Token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Data   []map[string]any `json:"data"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		err = json.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("esperado 200 na listagem, recebido %d", response.StatusCode)
		}
		if page.Limit != 1 {
			t.Errorf("limite esperado 1, recebido %d", page.Limit)
		}
		if offset == "0" {
			if page.Offset != 0 || len(page.Data) != 1 {
				t.Fatalf("esperada primeira página com um usuário: %+v", page)
			}
			if len(page.Data[0]) != 5 {
				t.Fatal("listagem deve conter apenas os cinco campos públicos")
			}
			for key, value := range body {
				if key == "createdAt" || key == "updatedAt" {
					createdTime, err := time.Parse(time.RFC3339Nano, value.(string))
					if err != nil {
						t.Fatal(err)
					}
					listedTime, err := time.Parse(time.RFC3339Nano, page.Data[0][key].(string))
					if err != nil {
						t.Fatal(err)
					}
					// PostgreSQL persists timestamps with microsecond precision.
					if !listedTime.Equal(createdTime.Truncate(time.Microsecond)) {
						t.Errorf("campo %s difere do instante persistido", key)
					}
					continue
				}
				if page.Data[0][key] != value {
					t.Errorf("campo %s difere do cadastro: %v", key, page.Data[0][key])
				}
			}
		} else if page.Offset != 1 || page.Data == nil || len(page.Data) != 0 {
			t.Fatalf("esperada página vazia após o último usuário: %+v", page)
		}
	}
}

func TestUserAccessEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	alice, err := users.Create(context.Background(), user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "Strong-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.Create(context.Background(), user.CreateInput{Name: "Bob", Email: "bob@example.com", Password: "Another-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newRouter(api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}), nil,
	))))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(method, path, payload, token string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if payload != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}
	assertStatus := func(want, got int, body []byte) {
		t.Helper()
		if got != want {
			t.Fatalf("expected HTTP %d, got %d: %s", want, got, body)
		}
	}
	login := func(email, password string) string {
		t.Helper()
		status, body := call(http.MethodPost, "/sessions", `{"email":"`+email+`","password":"`+password+`"}`, "")
		assertStatus(http.StatusOK, status, body)
		var session struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(body, &session); err != nil || session.Token == "" {
			t.Fatalf("invalid session: %s (%v)", body, err)
		}
		return session.Token
	}
	aliceToken := login("alice@example.com", "Strong-pass-123!")
	bobToken := login("bob@example.com", "Another-pass-123!")

	status, body := call(http.MethodGet, "/users", "", "")
	assertStatus(http.StatusUnauthorized, status, body)
	status, body = call(http.MethodPatch, "/users/"+alice.ID.String(), `{"name":"Intruder"}`, "")
	assertStatus(http.StatusUnauthorized, status, body)
	status, body = call(http.MethodGet, "/users", "", "invalid-token")
	assertStatus(http.StatusUnauthorized, status, body)

	for _, tt := range []struct {
		token        string
		owner, other user.User
	}{
		{aliceToken, alice, bob},
		{bobToken, bob, alice},
	} {
		status, body = call(http.MethodGet, "/users", "", tt.token)
		assertStatus(http.StatusOK, status, body)
		var page struct {
			Data   []map[string]any `json:"data"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if page.Limit != 20 || page.Offset != 0 || len(page.Data) != 1 || len(page.Data[0]) != 5 || page.Data[0]["id"] != tt.owner.ID.String() || strings.Contains(string(body), tt.other.Email) || strings.Contains(string(body), tt.owner.PasswordHash) {
			t.Fatalf("profile leaked or malformed: %s", body)
		}
		status, body = call(http.MethodGet, "/users?limit=1&offset=1", "", tt.token)
		assertStatus(http.StatusOK, status, body)
		if err := json.Unmarshal(body, &page); err != nil || page.Data == nil || len(page.Data) != 0 || page.Limit != 1 || page.Offset != 1 {
			t.Fatalf("expected empty page after self: %s (%v)", body, err)
		}
		status, body = call(http.MethodPatch, "/users/"+tt.other.ID.String(), `{"name":"Intruder"}`, tt.token)
		assertStatus(http.StatusNotFound, status, body)
	}

	status, body = call(http.MethodPatch, "/users/"+alice.ID.String(), `{"name":"Alice updated"}`, aliceToken)
	assertStatus(http.StatusOK, status, body)
	var updated map[string]any
	if err := json.Unmarshal(body, &updated); err != nil || len(updated) != 5 || updated["id"] != alice.ID.String() || updated["name"] != "Alice updated" || strings.Contains(string(body), alice.PasswordHash) {
		t.Fatalf("unexpected self update: %s (%v)", body, err)
	}
	persisted, err := users.Get(context.Background(), alice.ID)
	if err != nil || persisted.Name != "Alice updated" || persisted.PasswordHash != "" {
		t.Fatalf("self update was not persisted: %+v (%v)", persisted, err)
	}
	unchanged, err := users.Get(context.Background(), bob.ID)
	if err != nil || unchanged.Name != "Bob" {
		t.Fatalf("other owner changed: %+v (%v)", unchanged, err)
	}
}

func TestConcurrentRegistrationOfSameEmail(t *testing.T) {
	db, _ := testutil.Database(t)
	service := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	server := httptest.NewServer(newRouter(api.NewServer(service)))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			response, err := client.Post(server.URL+"/users", "application/json", strings.NewReader(`{"name":"Kaian","email":"same@example.com","password":"Uma-senha-de-teste-123!"}`))
			if err != nil {
				t.Error(err)
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		})
	}
	close(start)
	workers.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatalf("resultados inesperados: %v", counts)
	}
}
