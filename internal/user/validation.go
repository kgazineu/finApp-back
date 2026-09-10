package user

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	MinPasswordLength = 15
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
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < MinPasswordLength || length > MaxPasswordLength {
		return ErrInvalidPassword
	}
	return nil
}
