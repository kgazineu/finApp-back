package user

import "errors"

var (
	ErrEmailAlreadyExists = errors.New("e-mail já cadastrado")
	ErrInvalidPassword    = errors.New("senha deve conter entre 15 e 128 caracteres")
)
