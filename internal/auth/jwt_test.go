package auth

import (
	"testing"
	"time"
)

func TestJWT_SignAndParse(t *testing.T) {
	secret := []byte("test-secret-of-thirty-two-chars!")
	now := time.Now().UTC()

	token, err := SignAccessToken(secret, 42, "user", now)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := ParseAccessToken(secret, token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("UserID = %d, want 42", claims.UserID)
	}
	if claims.Role != "user" {
		t.Fatalf("Role = %q, want user", claims.Role)
	}
	got := claims.ExpiresAt.Time
	want := now.Add(AccessTTL)
	if got.Sub(want) > time.Second || want.Sub(got) > time.Second {
		t.Fatalf("ExpiresAt = %v, want ~%v", got, want)
	}
}

func TestJWT_RejectsBadSignature(t *testing.T) {
	secret := []byte("test-secret-of-thirty-two-chars!")
	token, _ := SignAccessToken(secret, 1, "user", time.Now())
	_, err := ParseAccessToken([]byte("different-secret-of-thirty-twoo!"), token)
	if err == nil {
		t.Fatal("expected parse to reject wrong-secret token")
	}
}

func TestRefreshToken_HashRoundTrip(t *testing.T) {
	raw, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(raw) == 0 || len(hash) != 64 {
		t.Fatalf("raw=%q hash=%q", raw, hash)
	}
	if HashRefreshToken(raw) != hash {
		t.Fatal("HashRefreshToken inconsistent")
	}
}
