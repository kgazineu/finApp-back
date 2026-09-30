package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/user"
)

type fakeRepository struct {
	found                              user.User
	findErr                            error
	saveErr                            error
	sessionErr                         error
	sessionUser                        uuid.UUID
	sessionExpiry                      time.Time
	savedHash                          [32]byte
	savedUser                          uuid.UUID
	savedExpiry                        time.Time
	lookedUpHash                       [32]byte
	findCalls, saveCalls, sessionCalls int
}

func (r *fakeRepository) FindByEmail(_ context.Context, _ string) (user.User, error) {
	r.findCalls++
	return r.found, r.findErr
}
func (r *fakeRepository) SaveSession(_ context.Context, hash [32]byte, id uuid.UUID, expiry time.Time) error {
	r.saveCalls++
	r.savedHash, r.savedUser, r.savedExpiry = hash, id, expiry
	return r.saveErr
}
func (r *fakeRepository) FindSession(_ context.Context, hash [32]byte) (uuid.UUID, time.Time, error) {
	r.sessionCalls++
	r.lookedUpHash = hash
	return r.sessionUser, r.sessionExpiry, r.sessionErr
}

type fakeVerifier struct {
	valid       bool
	err         error
	plain, hash string
	calls       int
}

func (v *fakeVerifier) Verify(plain, hash string) (bool, error) {
	v.calls++
	v.plain, v.hash = plain, hash
	return v.valid, v.err
}

func TestLoginAndAuthenticate(t *testing.T) {
	id := uuid.New()
	r := &fakeRepository{found: user.User{ID: id, PasswordHash: "stored-hash"}, sessionUser: id}
	v := &fakeVerifier{valid: true}
	s := auth.NewService(r, v)
	before := time.Now()
	token, expiry, err := s.Login(context.Background(), "someone@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || strings.ContainsAny(token, "+/=") {
		t.Fatal("unexpected token encoding")
	}
	secret, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(secret) != 32 {
		t.Fatalf("unexpected token bytes: %v", err)
	}
	if r.savedHash != sha256.Sum256(secret) || r.savedUser != id || !r.savedExpiry.Equal(expiry) {
		t.Fatal("session not saved with the hash, user ID and expiry")
	}
	if expiry.Before(before.Add(24*time.Hour)) || expiry.After(time.Now().Add(24*time.Hour)) {
		t.Fatalf("unexpected expiry: %v", expiry)
	}
	if v.plain != "secret" || v.hash != "stored-hash" {
		t.Fatal("password verifier received incorrect arguments")
	}
	r.sessionExpiry = expiry
	got, err := s.Authenticate(context.Background(), token)
	if err != nil || got != id || r.lookedUpHash != r.savedHash {
		t.Fatalf("authentication failed: id=%v err=%v", got, err)
	}
	second, _, err := s.Login(context.Background(), "someone@example.com", "secret")
	if err != nil || second == token {
		t.Fatalf("tokens must be independently generated: %v", err)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, email, password string
		findErr               error
		valid                 bool
	}{
		{name: "unknown email", email: "missing@example.com", password: "secret", findErr: user.ErrUserNotFound},
		{name: "wrong password", email: "known@example.com", password: "wrong"},
		{name: "empty email", password: "secret"},
		{name: "empty password", email: "known@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRepository{found: user.User{PasswordHash: "hash"}, findErr: tc.findErr}
			v := &fakeVerifier{valid: tc.valid}
			token, expiry, err := auth.NewService(r, v).Login(context.Background(), tc.email, tc.password)
			if err != auth.ErrInvalidCredentials || token != "" || !expiry.IsZero() || r.saveCalls != 0 {
				t.Fatalf("unexpected credential result: %v", err)
			}
		})
	}
}

func TestAuthenticateRejectsMalformedAndExpiredTokens(t *testing.T) {
	secret := make([]byte, 32)
	valid := base64.RawURLEncoding.EncodeToString(secret)
	r := &fakeRepository{sessionUser: uuid.New(), sessionExpiry: time.Now().Add(time.Hour)}
	s := auth.NewService(r, &fakeVerifier{})
	for _, token := range []string{"", "Bearer " + valid, "bearer " + valid, valid + "=", valid + " ", strings.Repeat("!", 43), valid[:42], valid[:42] + "B"} {
		_, err := s.Authenticate(context.Background(), token)
		if err != auth.ErrInvalidSession {
			t.Errorf("expected invalid session for malformed token, got %v", err)
		}
	}
	if r.sessionCalls != 0 {
		t.Fatal("malformed tokens must not hit the database")
	}
	for _, expiry := range []time.Time{time.Now().Add(-time.Second), time.Now().Add(-time.Hour)} {
		r.sessionExpiry = expiry
		if _, err := s.Authenticate(context.Background(), valid); err != auth.ErrInvalidSession {
			t.Fatalf("expired token accepted: %v", err)
		}
	}
	r.sessionErr = auth.ErrInvalidSession
	if _, err := s.Authenticate(context.Background(), valid); err != auth.ErrInvalidSession {
		t.Fatalf("missing session: %v", err)
	}
}

func TestAuthErrorsAndContext(t *testing.T) {
	failure := errors.New("database unavailable")
	r := &fakeRepository{found: user.User{ID: uuid.New()}, findErr: failure}
	v := &fakeVerifier{valid: true}
	s := auth.NewService(r, v)
	if _, _, err := s.Login(context.Background(), "a@example.com", "pass"); !errors.Is(err, failure) || err == failure {
		t.Fatalf("find error not wrapped: %v", err)
	}
	r.findErr = nil
	v.err = failure
	if _, _, err := s.Login(context.Background(), "a@example.com", "pass"); !errors.Is(err, failure) {
		t.Fatalf("verify error not propagated: %v", err)
	}
	v.err = nil
	r.saveErr = failure
	if token, expiry, err := s.Login(context.Background(), "a@example.com", "pass"); !errors.Is(err, failure) || err == failure || token != "" || !expiry.IsZero() {
		t.Fatalf("save failure leaked token or lost error: %v", err)
	}
	r.sessionErr = failure
	valid := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := s.Authenticate(context.Background(), valid); !errors.Is(err, failure) || err == failure {
		t.Fatalf("session lookup error not wrapped: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.Login(ctx, "a@example.com", "pass"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login: %v", err)
	}
	if _, err := s.Authenticate(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled authentication: %v", err)
	}
}
