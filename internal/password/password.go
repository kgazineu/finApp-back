package password

import (
	"github.com/alexedwards/argon2id"
)

type Hasher struct{}

func (Hasher) Hash(plain string) (string, error) {
	params := &argon2id.Params{
		Memory:      64 * 1024, // 64 MB
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}

	return argon2id.CreateHash(plain, params)
}

func (Hasher) Verify(plain, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plain, hash)
}
