package user

import "errors"

var (
	ErrEmailAlreadyExists = errors.New("e-mail já cadastrado")
	ErrInvalidPassword    = errors.New("senha deve conter entre 15 e 128 caracteres")
	ErrInvalidPagination  = errors.New("limit deve estar entre 1 e 100 e offset deve ser maior ou igual a zero")
)
