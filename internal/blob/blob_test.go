package blob

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFS_PutGetStat(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := s.Put(context.Background(), "images", "user-1", "abc.png", []byte("png-bytes"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if rel != filepath.Join("images", "user-1", "abc.png") {
		t.Fatalf("rel = %q", rel)
	}
	var buf bytes.Buffer
	if err := s.Get(context.Background(), rel, &buf); err != nil {
		t.Fatalf("get: %v", err)
	}
	if buf.String() != "png-bytes" {
		t.Fatalf("got %q", buf.String())
	}
	size, err := s.Stat(context.Background(), rel)
	if err != nil || size != 9 {
		t.Fatalf("stat: size=%d err=%v", size, err)
	}
}

func TestLocalFS_GetMissing(t *testing.T) {
	s, _ := NewLocalFS(t.TempDir())
	var buf bytes.Buffer
	err := s.Get(context.Background(), "nope/x.png", &buf)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalFS_DeleteIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalFS(dir)
	rel, _ := s.Put(context.Background(), "x", "u", "k", []byte("v"))
	if err := s.Delete(context.Background(), rel); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := s.Delete(context.Background(), rel); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: %v", err)
	}
}
