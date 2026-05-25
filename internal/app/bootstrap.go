// Package app bundles boot-time wiring shared across cmd/server and tests.
package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/liusx/shadraw/internal/auth"
	"github.com/liusx/shadraw/internal/user"
)

// EnsureAdmin promotes (or creates) the configured ADMIN_EMAIL user to the
// admin role. On first creation, a temporary password is generated and logged.
func EnsureAdmin(ctx context.Context, users *user.Repository, adminEmail string) error {
	existing, err := users.FindByEmail(ctx, adminEmail)
	if err == nil {
		if existing.Role != user.RoleAdmin {
			if err := users.SetRole(ctx, existing.ID, user.RoleAdmin); err != nil {
				return fmt.Errorf("promote admin: %w", err)
			}
			slog.Info("admin bootstrap: promoted existing user", "email", adminEmail)
		} else {
			slog.Info("admin bootstrap: existing admin verified", "email", adminEmail)
		}
		return nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return fmt.Errorf("lookup admin: %w", err)
	}

	tempPassword, err := randomPassword(16)
	if err != nil {
		return fmt.Errorf("generate temp password: %w", err)
	}
	hash, err := auth.HashPassword(tempPassword)
	if err != nil {
		return fmt.Errorf("hash temp password: %w", err)
	}
	u := &user.User{
		Email:              adminEmail,
		PasswordHash:       hash,
		DisplayName:        "admin",
		Role:               user.RoleAdmin,
		MustChangePassword: true,
	}
	if err := users.Create(ctx, u); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	slog.Warn("ADMIN BOOTSTRAP — temporary password printed ONCE; change immediately after first login",
		"email", adminEmail, "tempPassword", tempPassword)
	return nil
}

// randomPassword returns a base64-encoded random string of approximately
// `chars` characters in length.
func randomPassword(chars int) (string, error) {
	// base64 grows 4 bytes per 3 input bytes; ceil to satisfy `chars`.
	rawLen := (chars*3 + 3) / 4
	b := make([]byte, rawLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString(b)
	if len(s) > chars {
		s = s[:chars]
	}
	return s, nil
}
