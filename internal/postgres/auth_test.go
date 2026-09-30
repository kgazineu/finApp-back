package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/internal/user"
)

func TestAuthRepositoryLoginRoundTrip(t *testing.T) {
	db, _ := testutil.Database(t)
	ctx := context.Background()
	hash, err := (password.Hasher{}).Hash("correct password")
	if err != nil {
		t.Fatal(err)
	}
	u, err := user.New("Test", uuid.NewString()+"@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewUserRepository(db).Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewAuthRepository(db)
	found, err := repo.FindByEmail(ctx, u.Email)
	if err != nil || found.ID != u.ID || found.PasswordHash != hash {
		t.Fatalf("find by email failed: %v", err)
	}
	if _, err := repo.FindByEmail(ctx, "missing@example.com"); !errors.Is(err, user.ErrUserNotFound) {
		t.Fatalf("missing user: %v", err)
	}
	svc := auth.NewService(repo, password.Hasher{})
	if _, _, err := svc.Login(ctx, u.Email, "wrong password"); err != auth.ErrInvalidCredentials {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, err := svc.Login(ctx, "missing@example.com", "correct password"); err != auth.ErrInvalidCredentials {
		t.Fatalf("unknown email: %v", err)
	}
	token, expiry, err := svc.Login(ctx, u.Email, "correct password")
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Authenticate(ctx, token)
	if err != nil || id != u.ID {
		t.Fatalf("session roundtrip: id=%s err=%v", id, err)
	}
	var storedHash []byte
	var storedUser uuid.UUID
	var storedExpiry, createdAt time.Time
	if err := db.Raw("SELECT token_hash, user_id, expires_at, created_at FROM sessions").Row().Scan(
		&storedHash, &storedUser, &storedExpiry, &createdAt,
	); err != nil {
		t.Fatal(err)
	}
	if len(storedHash) != 32 || storedUser != u.ID || !storedExpiry.Equal(expiry.Truncate(time.Microsecond)) || createdAt.IsZero() {
		t.Fatal("incorrect session persistence")
	}
	secret, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || sha256.Sum256(secret) != [32]byte(storedHash) || string(secret) == string(storedHash) {
		t.Fatalf("session hash mismatch: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token[:42]+"B"); err != auth.ErrInvalidSession {
		t.Fatalf("malformed token: %v", err)
	}
	if err := db.Exec("UPDATE sessions SET expires_at = ? WHERE token_hash = ?", time.Now().Add(-time.Second), storedHash).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); err != auth.ErrInvalidSession {
		t.Fatalf("expired session: %v", err)
	}
}

func TestAuthRepositorySessionConstraints(t *testing.T) {
	db, _ := testutil.Database(t)
	ctx := context.Background()
	repo := postgres.NewAuthRepository(db)
	var hash [32]byte
	if _, _, err := repo.FindSession(ctx, hash); err != auth.ErrInvalidSession {
		t.Fatalf("missing session: %v", err)
	}
	if err := repo.SaveSession(ctx, hash, uuid.New(), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("session accepted unknown user")
	}
	owner, err := user.New("Test", uuid.NewString()+"@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.NewUserRepository(db).Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)", []byte{1}, owner.ID, time.Now()).Error; err == nil {
		t.Fatal("session accepted short hash")
	}
	if err := repo.SaveSession(ctx, hash, owner.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSession(ctx, hash, owner.ID, time.Now().Add(time.Hour)); err == nil {
		t.Fatal("session accepted duplicate hash")
	}
}

func TestAuthRepositoryCanceledContext(t *testing.T) {
	db, _ := testutil.Database(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := postgres.NewAuthRepository(db)
	if _, err := repo.FindByEmail(ctx, "a@example.com"); !errors.Is(err, context.Canceled) {
		t.Fatalf("find by email: %v", err)
	}
	if err := repo.SaveSession(ctx, [32]byte{}, uuid.New(), time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("save: %v", err)
	}
	if _, _, err := repo.FindSession(ctx, [32]byte{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("find session: %v", err)
	}
}
