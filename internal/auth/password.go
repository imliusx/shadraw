package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost balances security and login latency. 12 ≈ 150–250ms on commodity hardware.
const bcryptCost = 12

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether plain matches the bcrypt hash. Returns nil
// on match, ErrPasswordMismatch on mismatch, or another error on internal issues.
var ErrPasswordMismatch = errors.New("password mismatch")

func VerifyPassword(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}
	return err
}
