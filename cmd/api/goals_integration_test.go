package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestGoalFlowEndToEnd(t *testing.T) {
	db, _ := testutil.Database(t)
	users := user.NewService(postgres.NewUserRepository(db), password.Hasher{})
	alice, err := users.Create(context.Background(), user.CreateInput{Name: "Alice", Email: "goal-alice@example.com", Password: "Strong-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.Create(context.Background(), user.CreateInput{Name: "Bob", Email: "goal-bob@example.com", Password: "Another-pass-123!"})
	if err != nil {
		t.Fatal(err)
	}
	router := newRouter(api.NewServer(users, api.WithFinancialServices(
		auth.NewService(postgres.NewAuthRepository(db), password.Hasher{}),
		transaction.NewService(postgres.NewTransactionRepository(db)),
	), api.WithGoalService(goal.NewService(postgres.NewGoalRepository(db)))))

	login := func(email, pass string) string {
		t.Helper()
		body, err := json.Marshal(map[string]string{"email": email, "password": pass})
		if err != nil {
			t.Fatal(err)
		}
		response := goalRequest(t, router, http.MethodPost, "/sessions", string(body), "")
		if response.Code != http.StatusOK {
			t.Fatalf("login HTTP %d: %s", response.Code, response.Body.String())
		}
		var session struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || session.Token == "" {
			t.Fatalf("invalid session: %s (%v)", response.Body.String(), err)
		}
		return "Bearer " + session.Token
	}
	aliceToken := login("goal-alice@example.com", "Strong-pass-123!")
	bobToken := login("goal-bob@example.com", "Another-pass-123!")

	type goalDTO struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		TargetMinor int64     `json:"targetMinor"`
		SavedMinor  int64     `json:"savedMinor"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}
	decodeGoal := func(data []byte) goalDTO {
		t.Helper()
		var item goalDTO
		if err := json.Unmarshal(data, &item); err != nil {
			t.Fatal(err)
		}
		if item.ID == uuid.Nil || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
			t.Fatalf("invalid goal response: %s", data)
		}
		return item
	}
	create := func(token, name string, target int64) goalDTO {
		t.Helper()
		body, err := json.Marshal(map[string]any{"name": name, "targetMinor": target})
		if err != nil {
			t.Fatal(err)
		}
		response := goalRequest(t, router, http.MethodPost, "/goals", string(body), token)
		if response.Code != http.StatusCreated {
			t.Fatalf("create HTTP %d: %s", response.Code, response.Body.String())
		}
		item := decodeGoal(response.Body.Bytes())
		if item.Name != name || item.TargetMinor != target || item.SavedMinor != 0 {
			t.Fatalf("unexpected created goal: %+v", item)
		}
		return item
	}
	list := func(token, path string, limit, offset int) []goalDTO {
		t.Helper()
		response := goalRequest(t, router, http.MethodGet, path, "", token)
		if response.Code != http.StatusOK {
			t.Fatalf("list HTTP %d: %s", response.Code, response.Body.String())
		}
		var page struct {
			Data   []goalDTO `json:"data"`
			Limit  int       `json:"limit"`
			Offset int       `json:"offset"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Data == nil || page.Limit != limit || page.Offset != offset {
			t.Fatalf("unexpected page: %+v", page)
		}
		return page.Data
	}
	adjust := func(token string, id uuid.UUID, direction string, amount int64, status int) goalDTO {
		t.Helper()
		body, err := json.Marshal(map[string]any{"direction": direction, "amountMinor": amount})
		if err != nil {
			t.Fatal(err)
		}
		response := goalRequest(t, router, http.MethodPost, "/goals/"+id.String()+"/allocations", string(body), token)
		if status != http.StatusOK {
			assertGoalError(t, response, status)
			return goalDTO{}
		}
		if response.Code != status {
			t.Fatalf("adjust HTTP %d: %s", response.Code, response.Body.String())
		}
		return decodeGoal(response.Body.Bytes())
	}

	aliceFirst := create(aliceToken, "Trip", 10000)
	aliceSecond := create(aliceToken, "Emergency", 20000)
	bobGoal := create(bobToken, "Bob's goal", 30000)
	var persistedOwner uuid.UUID
	if err := db.Raw("SELECT user_id FROM goals WHERE id = ?", aliceFirst.ID).Row().Scan(&persistedOwner); err != nil {
		t.Fatal(err)
	}
	if persistedOwner != alice.ID || persistedOwner == bob.ID {
		t.Fatalf("created goal assigned to %s, want %s", persistedOwner, alice.ID)
	}

	aliceGoals := list(aliceToken, "/goals", goal.DefaultListLimit, 0)
	if len(aliceGoals) != 2 {
		t.Fatalf("Alice sees %d goals, want 2: %+v", len(aliceGoals), aliceGoals)
	}
	seen := map[uuid.UUID]bool{}
	for _, item := range aliceGoals {
		seen[item.ID] = true
	}
	if !seen[aliceFirst.ID] || !seen[aliceSecond.ID] || seen[bobGoal.ID] {
		t.Fatalf("Alice's list exposed another owner or omitted her goal: %+v", aliceGoals)
	}
	for offset, want := range aliceGoals {
		page := list(aliceToken, "/goals?limit=1&offset="+strconv.Itoa(offset), 1, offset)
		if len(page) != 1 || page[0].ID != want.ID {
			t.Fatalf("page %d: %+v, want %s", offset, page, want.ID)
		}
	}
	if page := list(aliceToken, "/goals?limit=1&offset=2", 1, 2); len(page) != 0 {
		t.Fatalf("expected empty page, got %+v", page)
	}
	bobGoals := list(bobToken, "/goals", goal.DefaultListLimit, 0)
	if len(bobGoals) != 1 || bobGoals[0].ID != bobGoal.ID {
		t.Fatalf("Bob's list exposed another owner: %+v", bobGoals)
	}

	adjust(bobToken, aliceFirst.ID, "deposit", 10, http.StatusNotFound)
	adjust(bobToken, aliceFirst.ID, "withdraw", 10, http.StatusNotFound)
	adjust(aliceToken, bobGoal.ID, "deposit", 10, http.StatusNotFound)
	adjust(aliceToken, aliceFirst.ID, "withdraw", 1, http.StatusBadRequest)
	deposited := adjust(aliceToken, aliceFirst.ID, "deposit", 500, http.StatusOK)
	if deposited.ID != aliceFirst.ID || deposited.SavedMinor != 500 || deposited.TargetMinor != aliceFirst.TargetMinor || deposited.Name != aliceFirst.Name {
		t.Fatalf("unexpected deposited goal: %+v", deposited)
	}
	adjust(bobToken, aliceFirst.ID, "withdraw", 100, http.StatusNotFound)
	adjust(bobToken, aliceFirst.ID, "deposit", 100, http.StatusNotFound)
	adjust(aliceToken, aliceFirst.ID, "withdraw", 501, http.StatusBadRequest)
	withdrawn := adjust(aliceToken, aliceFirst.ID, "withdraw", 175, http.StatusOK)
	if withdrawn.ID != aliceFirst.ID || withdrawn.SavedMinor != 325 {
		t.Fatalf("unexpected withdrawn goal: %+v", withdrawn)
	}
	aliceGoals = list(aliceToken, "/goals", goal.DefaultListLimit, 0)
	for _, item := range aliceGoals {
		if item.ID == aliceFirst.ID && item.SavedMinor != 325 || item.ID == aliceSecond.ID && item.SavedMinor != 0 {
			t.Fatalf("unexpected saved balance: %+v", aliceGoals)
		}
	}
	bobGoals = list(bobToken, "/goals", goal.DefaultListLimit, 0)
	if len(bobGoals) != 1 || bobGoals[0].ID != bobGoal.ID || bobGoals[0].SavedMinor != 0 {
		t.Fatalf("other owner's balance changed: %+v", bobGoals)
	}
}
