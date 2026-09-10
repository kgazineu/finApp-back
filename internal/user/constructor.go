package user

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidName         = errors.New("nome não pode ficar vazio")
	ErrInvalidEmail        = errors.New("e-mail inválido")
	ErrInvalidPasswordHash = errors.New("hash de senha não pode ficar vazio")
)

func New(name, email, passwordHash string) (User, error) {
	if strings.TrimSpace(name) == "" {
		return User{}, ErrInvalidName
	}

	address, err := mail.ParseAddress(email)
	if err != nil {
		return User{}, ErrInvalidEmail
	}

	if address.Address != email {
		return User{}, ErrInvalidEmail
	}

	if strings.TrimSpace(passwordHash) == "" {
		return User{}, ErrInvalidPasswordHash
	}

	id, err := uuid.NewRandom()

	now := time.Now().UTC()

	return User{
		ID:           id,
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
