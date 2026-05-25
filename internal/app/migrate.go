package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all up migrations under migrationsDir against dsn.
// Idempotent: returns nil when no change is needed.
func RunMigrations(ctx context.Context, migrationsDir, dsn string) error {
	m, err := migrate.New("file://"+migrationsDir, dsn)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- m.Up() }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migrate up: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
