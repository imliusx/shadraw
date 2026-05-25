// Package crypto provides authenticated encryption for sensitive config values
// (currently the upstream provider apiKey). The master key comes from env
// MASTER_KEY as 32 bytes base64-encoded.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// AESGCM wraps a 256-bit AES-GCM cipher keyed by the configured master key.
type AESGCM struct {
	aead cipher.AEAD
}

// New parses masterKeyB64 (base64 encoded 32-byte key) and returns a usable cipher.
func New(masterKeyB64 string) (*AESGCM, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("master key not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &AESGCM{aead: aead}, nil
}

// Encrypt seals plaintext into a ciphertext blob: [nonce | sealed]. Returns []byte
// so callers can persist it as bytea.
func (c *AESGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt reverses Encrypt.
func (c *AESGCM) Decrypt(blob []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, sealed := blob[:ns], blob[ns:]
	return c.aead.Open(nil, nonce, sealed, nil)
}

// MaskAPIKey returns a display-safe preview of an apiKey: first 4 chars +
// `***` + last 4 chars. For very short keys it returns just `***`.
func MaskAPIKey(plain string) string {
	if len(plain) < 12 {
		return "***"
	}
	return plain[:4] + "***" + plain[len(plain)-4:]
}
