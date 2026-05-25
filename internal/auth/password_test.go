package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_VerifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter22ok")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", hash)
	}
	if err := VerifyPassword(hash, "hunter22ok"); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyPassword_Mismatch(t *testing.T) {
	hash, _ := HashPassword("right-one")
	err := VerifyPassword(hash, "wrong-one")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if err != ErrPasswordMismatch {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}
