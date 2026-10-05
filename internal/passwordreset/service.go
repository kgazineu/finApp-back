package passwordreset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
)

const (
	codeLifetime = 15 * time.Minute
	// cada verificação (certa ou errada) gasta uma tentativa: verificar e confirmar usam duas
	maxAttempts = 5
)

var ErrInvalidCode = errors.New("código inválido ou expirado")

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Hasher interface {
	Hash(plain string) (string, error)
}

type Service struct {
	repo   *Repository
	mailer Mailer
	hasher Hasher
}

func NewService(repo *Repository, mailer Mailer, hasher Hasher) *Service {
	return &Service{repo: repo, mailer: mailer, hasher: hasher}
}

// Request mails a new code. It succeeds silently for unknown e-mails (no account enumeration)
// and while the previous code is younger than one minute (no mail flooding).
func (s *Service) Request(ctx context.Context, email string) error {
	code, err := newCode()
	if err != nil {
		return err
	}

	created, err := s.repo.Upsert(ctx, email, hashCode(code), time.Now().Add(codeLifetime))
	if err != nil || !created {
		return err
	}

	// envia fora da requisição: o tempo de resposta não revela se o e-mail existe
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		body := fmt.Sprintf("Seu código de recuperação de senha do FinApp é %s.\n\nEle vale por %d minutos. Se você não pediu, ignore este e-mail.", code, int(codeLifetime.Minutes()))
		if err := s.mailer.Send(ctx, email, "Código de recuperação de senha", body); err != nil {
			slog.Error("falha ao enviar código de recuperação", "error", err)
		}
	}()

	return nil
}

func (s *Service) Verify(ctx context.Context, email, code string) error {
	_, err := s.check(ctx, email, code)
	return err
}

func (s *Service) Confirm(ctx context.Context, email, code, password string) error {
	// valida a senha antes de gastar uma tentativa do código
	if err := user.ValidatePassword(password); err != nil {
		return err
	}

	userID, err := s.check(ctx, email, code)
	if err != nil {
		return err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("gerar hash da senha: %w", err)
	}

	return s.repo.ResetPassword(ctx, userID, hash)
}

func (s *Service) check(ctx context.Context, email, code string) (uuid.UUID, error) {
	userID, stored, err := s.repo.UseAttempt(ctx, email, maxAttempts)
	if err != nil {
		return uuid.Nil, err
	}

	given := hashCode(code)
	if stored == nil || subtle.ConstantTimeCompare(stored, given[:]) != 1 {
		return uuid.Nil, ErrInvalidCode
	}

	return userID, nil
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("gerar código: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashCode(code string) [32]byte {
	return sha256.Sum256([]byte(code))
}
