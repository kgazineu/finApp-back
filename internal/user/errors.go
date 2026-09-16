package user

import (
	"errors"
	"strings"
)

var (
	ErrEmailAlreadyExists = errors.New("e-mail já cadastrado")
	ErrInvalidPassword    = errors.New("senha inválida")
	ErrInvalidPagination  = errors.New("limit deve estar entre 1 e 100 e offset deve ser maior ou igual a zero")
)

type PasswordValidationError struct {
	Missing []string
}

func (e *PasswordValidationError) Error() string {
	const prefix = "A senha precisa conter "
	if len(e.Missing) == 0 {
		return ErrInvalidPassword.Error()
	}
	if len(e.Missing) == 1 {
		return prefix + e.Missing[0]
	}
	return prefix + strings.Join(e.Missing[:len(e.Missing)-1], ", ") + " e " + e.Missing[len(e.Missing)-1]
}

func (*PasswordValidationError) Unwrap() error {
	return ErrInvalidPassword
}
