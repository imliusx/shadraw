package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k[:])
}

func TestAESGCM_RoundTrip(t *testing.T) {
	c, err := New(newKey(t))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	plain := []byte("sk-test-1234567890abcdef")
	blob, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := c.Decrypt(blob)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plain mismatch: got %q want %q", got, plain)
	}
}

func TestAESGCM_DistinctNonces(t *testing.T) {
	c, _ := New(newKey(t))
	a, _ := c.Encrypt([]byte("hello"))
	b, _ := c.Encrypt([]byte("hello"))
	if bytes.Equal(a, b) {
		t.Fatal("encryption of identical plaintexts must differ (random nonce)")
	}
}

func TestAESGCM_BadKey(t *testing.T) {
	if _, err := New("not-base64!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if _, err := New("c2hvcnQ="); err == nil { // "short" base64
		t.Fatal("expected error for short key")
	}
}

func TestMaskAPIKey(t *testing.T) {
	if got := MaskAPIKey("sk-1234567890abcd"); got != "sk-1***abcd" {
		t.Fatalf("got %q", got)
	}
	if got := MaskAPIKey("short"); got != "***" {
		t.Fatalf("got %q", got)
	}
}
