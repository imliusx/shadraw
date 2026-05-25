// Package blob provides a small storage abstraction for binary assets
// (currently generated images). The local-filesystem implementation writes
// atomically under DATA_DIR/<bucket>/<user>/<key>.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store is the persistence boundary for binary blobs.
type Store interface {
	// Put atomically writes data and returns the relative path (relative to the
	// data root) for storage in the database.
	Put(ctx context.Context, bucket, userKey, fileKey string, data []byte) (string, error)
	// Get streams the blob into w. Returns ErrNotFound if missing.
	Get(ctx context.Context, relPath string, w io.Writer) error
	// Delete removes a blob; missing files are treated as success (idempotent).
	Delete(ctx context.Context, relPath string) error
	// Stat returns size in bytes; returns ErrNotFound if missing.
	Stat(ctx context.Context, relPath string) (int64, error)
}

// ErrNotFound is returned by Get/Stat when the path doesn't exist.
var ErrNotFound = errors.New("blob not found")

// LocalFS is the dev/single-host implementation. Files live under rootDir.
type LocalFS struct {
	rootDir string
}

// NewLocalFS prepares a Store that writes under rootDir (created if missing).
func NewLocalFS(rootDir string) (*LocalFS, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir root: %w", err)
	}
	return &LocalFS{rootDir: rootDir}, nil
}

// rel returns the path relative to rootDir for DB storage.
func (l *LocalFS) rel(bucket, userKey, fileKey string) string {
	return filepath.Join(bucket, userKey, fileKey)
}

// abs returns the absolute on-disk path.
func (l *LocalFS) abs(relPath string) string {
	return filepath.Join(l.rootDir, relPath)
}

func (l *LocalFS) Put(_ context.Context, bucket, userKey, fileKey string, data []byte) (string, error) {
	rel := l.rel(bucket, userKey, fileKey)
	abs := l.abs(rel)
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".put-*.tmp")
	if err != nil {
		return "", fmt.Errorf("temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// best-effort cleanup if rename failed
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return "", fmt.Errorf("rename: %w", err)
	}
	return rel, nil
}

func (l *LocalFS) Get(_ context.Context, relPath string, w io.Writer) error {
	f, err := os.Open(l.abs(relPath))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func (l *LocalFS) Delete(_ context.Context, relPath string) error {
	err := os.Remove(l.abs(relPath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalFS) Stat(_ context.Context, relPath string) (int64, error) {
	st, err := os.Stat(l.abs(relPath))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return st.Size(), nil
}
