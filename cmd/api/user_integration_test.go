package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestUserRegistrationEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	service := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	server := httptest.NewServer(newRouter(api.NewServer(service)))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	payload := `{"name":"Kaian","email":"kaian@example.com","password":"uma-senha-de-teste-123!"}`
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
	if hash == "uma-senha-de-teste-123!" {
		t.Fatal("senha persistida em texto puro")
	}
	match, err := (password.Hasher{}).Verify("uma-senha-de-teste-123!", hash)
	if err != nil || !match {
		t.Fatal("hash persistido não verifica a senha original")
	}
	for _, tt := range []struct {
		body   string
		status int
	}{
		{payload, 409},
		{`{"name":"Kaian","email":"outro@example.com","password":"curta"}`, 400},
		{`{"name":"   ","email":"outro@example.com","password":"uma-senha-de-teste-123!"}`, 400},
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
			response, err := client.Post(server.URL+"/users", "application/json", strings.NewReader(`{"name":"Kaian","email":"same@example.com","password":"uma-senha-de-teste-123!"}`))
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
