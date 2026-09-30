package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
)

const sessionLifetime = 24 * time.Hour

type Repository interface {
	FindByEmail(ctx context.Context, email string) (user.User, error)
	SaveSession(ctx context.Context, tokenHash [32]byte, userID uuid.UUID, expiresAt time.Time) error
	FindSession(ctx context.Context, tokenHash [32]byte) (uuid.UUID, time.Time, error)
}

type PasswordVerifier interface {
	Verify(plain, hash string) (bool, error)
}

type Service struct {
	repo     Repository
	verifier PasswordVerifier
}

func NewService(repo Repository, verifier PasswordVerifier) *Service {
	return &Service{repo: repo, verifier: verifier}
}

func (s *Service) Login(ctx context.Context, email, password string) (token string, expiresAt time.Time, err error) {
	if err := ctx.Err(); err != nil {
		return "", time.Time{}, err
	}
	if email == "" || password == "" {
		return "", time.Time{}, ErrInvalidCredentials
	}

	u, err := s.repo.FindByEmail(ctx, email)
	if errors.Is(err, user.ErrUserNotFound) {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("find login user: %w", err)
	}
	valid, err := s.verifier.Verify(password, u.PasswordHash)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return "", time.Time{}, err
	}

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(secret[:])
	expiresAt = time.Now().UTC().Add(sessionLifetime)
	if err := s.repo.SaveSession(ctx, sha256.Sum256(secret[:]), u.ID, expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("save session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, err
	}
	// Exactly 32 bytes in unpadded base64url; reject headers, padded and noncanonical encodings.
	if len(token) != 43 {
		return uuid.Nil, ErrInvalidSession
	}
	secret, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(secret) != 32 || base64.RawURLEncoding.EncodeToString(secret) != token {
		return uuid.Nil, ErrInvalidSession
	}
	userID, expiresAt, err := s.repo.FindSession(ctx, sha256.Sum256(secret))
	if errors.Is(err, ErrInvalidSession) {
		return uuid.Nil, ErrInvalidSession
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("find session: %w", err)
	}
	if !time.Now().Before(expiresAt) {
		return uuid.Nil, ErrInvalidSession
	}
	return userID, nil
}
