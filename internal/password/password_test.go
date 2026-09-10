package password_test

import (
	"testing"

	"github.com/kgazineu/finApp-back/internal/password"
)

func TestHashAndVerify(t *testing.T) {
	hasher := password.Hasher{}
	plain := "uma-senha-de-teste-123!"

	hash, err := hasher.Hash(plain)
	if err != nil {
		t.Fatalf("erro inesperado ao gerar hash: %v", err)
	}

	if hash == "" {
		t.Fatal("hash não pode ficar vazio")
	}

	if hash == plain {
		t.Fatal("hash não pode ser igual à senha original")
	}

	match, err := hasher.Verify(plain, hash)
	if err != nil {
		t.Fatalf("erro inesperado ao verificar senha: %v", err)
	}

	if !match {
		t.Error("a senha original deveria corresponder ao hash")
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	hasher := password.Hasher{}

	hash, err := hasher.Hash("senha-original-123!")
	if err != nil {
		t.Fatalf("erro inesperado ao gerar hash: %v", err)
	}

	match, err := hasher.Verify("outra-senha-456!", hash)
	if err != nil {
		t.Fatalf("erro inesperado ao verificar senha: %v", err)
	}

	if match {
		t.Error("uma senha diferente não deveria corresponder ao hash")
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	hasher := password.Hasher{}
	plain := "uma-senha-de-teste-123!"

	first, err := hasher.Hash(plain)
	if err != nil {
		t.Fatalf("erro ao gerar primeiro hash: %v", err)
	}

	second, err := hasher.Hash(plain)
	if err != nil {
		t.Fatalf("erro ao gerar segundo hash: %v", err)
	}

	if first == second {
		t.Fatal("gerações independentes deveriam produzir hashes diferentes")
	}

	for _, hash := range []string{first, second} {
		match, err := hasher.Verify(plain, hash)
		if err != nil {
			t.Fatalf("erro ao verificar hash: %v", err)
		}

		if !match {
			t.Error("a senha original deveria corresponder a ambos os hashes")
		}
	}
}
