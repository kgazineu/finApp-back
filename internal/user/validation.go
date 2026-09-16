package user

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MinPasswordLength = 8
	MaxPasswordLength = 128
)

func validateProfile(name, email string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) {
		return ErrInvalidName
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return ErrInvalidEmail
	}
	return nil
}

func validatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrInvalidPassword
	}

	length := utf8.RuneCountInString(password)
	if length > MaxPasswordLength {
		return ErrInvalidPassword
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		hasUpper = hasUpper || unicode.IsUpper(char)
		hasLower = hasLower || unicode.IsLower(char)
		hasNumber = hasNumber || unicode.IsDigit(char)
		hasSpecial = hasSpecial || unicode.IsPunct(char) || unicode.IsSymbol(char)
	}

	missing := make([]string, 0, 5)
	if length < MinPasswordLength {
		missing = append(missing, "pelo menos 8 caracteres")
	}
	if !hasUpper {
		missing = append(missing, "uma letra maiúscula")
	}
	if !hasLower {
		missing = append(missing, "uma letra minúscula")
	}
	if !hasNumber {
		missing = append(missing, "um número")
	}
	if !hasSpecial {
		missing = append(missing, "um caractere especial")
	}
	if len(missing) > 0 {
		return &PasswordValidationError{Missing: missing}
	}
	return nil
}
