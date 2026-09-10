package user

import (
	"errors"
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
	if err := validateProfile(name, email); err != nil {
		return User{}, err
	}

	if strings.TrimSpace(passwordHash) == "" {
		return User{}, ErrInvalidPasswordHash
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return User{}, err
	}

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
