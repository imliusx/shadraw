package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStore struct {
	paths []string
	sizes map[string]int64
}

func (f *fakeStore) Put(_ context.Context, bucket, userKey, fileKey string, _ []byte) (string, error) {
	rel := bucket + "/" + userKey + "/" + fileKey
	f.paths = append(f.paths, rel)
	return rel, nil
}

func (f *fakeStore) Get(_ context.Context, _ string, _ io.Writer) error {
	panic("unexpected Get")
}

func (f *fakeStore) Delete(_ context.Context, _ string) error {
	panic("unexpected Delete")
}

func (f *fakeStore) Stat(_ context.Context, _ string) (int64, error) {
	panic("unexpected Stat")
}

type fakeVerifyStore struct {
	sizes map[string]int64
}

func (f *fakeVerifyStore) Put(_ context.Context, _, _, _ string, _ []byte) (string, error) {
	panic("unexpected Put")
}

func (f *fakeVerifyStore) Get(_ context.Context, _ string, _ io.Writer) error {
	panic("unexpected Get")
}

func (f *fakeVerifyStore) Delete(_ context.Context, _ string) error {
	panic("unexpected Delete")
}

func (f *fakeVerifyStore) Stat(_ context.Context, relPath string) (int64, error) {
	size, ok := f.sizes[relPath]
	if !ok {
		return 0, os.ErrNotExist
	}
	return size, nil
}

func TestSplitObjectKey(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantBucket string
		wantUser   string
		wantFile   string
		wantErr    bool
	}{
		{
			name:       "standard image key",
			key:        "images/user-13/abc.png",
			wantBucket: "images",
			wantUser:   "user-13",
			wantFile:   "abc.png",
		},
		{
			name:       "nested file key",
			key:        "images/user-13/nested/abc.png",
			wantBucket: "images",
			wantUser:   "user-13",
			wantFile:   "nested/abc.png",
		},
		{name: "wrong bucket", key: "avatars/user-13/abc.png", wantErr: true},
		{name: "missing user", key: "images//abc.png", wantErr: true},
		{name: "missing file", key: "images/user-13", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBucket, gotUser, gotFile, err := splitObjectKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitObjectKey() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("splitObjectKey() error = %v", err)
			}
			if gotBucket != tt.wantBucket || gotUser != tt.wantUser || gotFile != tt.wantFile {
				t.Fatalf("splitObjectKey() = (%q, %q, %q), want (%q, %q, %q)",
					gotBucket, gotUser, gotFile, tt.wantBucket, tt.wantUser, tt.wantFile)
			}
		})
	}
}

func TestMigrate_UploadsImagesAndSkipsHiddenFiles(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "images", "user-13", "a.png"), []byte("aaa"))
	writeFile(t, filepath.Join(dataDir, "images", ".DS_Store"), []byte("metadata"))
	writeFile(t, filepath.Join(dataDir, "images", ".hidden", "b.png"), []byte("bbb"))

	store := &fakeStore{}
	got, err := migrate(context.Background(), options{dataDir: dataDir}, store)
	if err != nil {
		t.Fatalf("migrate() error = %v", err)
	}
	if got.scanned != 2 || got.uploaded != 1 || got.skipped != 2 || got.failed != 0 || got.bytes != 3 {
		t.Fatalf("migrate() stats = %+v", got)
	}
	if len(store.paths) != 1 || store.paths[0] != "images/user-13/a.png" {
		t.Fatalf("uploaded paths = %v", store.paths)
	}
}

func TestMigrate_VerifyOnlyChecksRemoteSizes(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "images", "user-13", "a.png"), []byte("aaa"))
	writeFile(t, filepath.Join(dataDir, "images", "user-13", "b.png"), []byte("b"))

	store := &fakeVerifyStore{sizes: map[string]int64{
		"images/user-13/a.png": 3,
		"images/user-13/b.png": 2,
	}}
	got, err := migrate(context.Background(), options{dataDir: dataDir, verifyOnly: true}, store)
	if err != nil {
		t.Fatalf("migrate() error = %v", err)
	}
	if got.scanned != 2 || got.verified != 1 || got.failed != 1 {
		t.Fatalf("migrate() stats = %+v", got)
	}
}

func TestValidate_RequiresS3ConfigUnlessDryRun(t *testing.T) {
	if err := validate(options{dataDir: "./data", dryRun: true}); err != nil {
		t.Fatalf("validate(dryRun) error = %v", err)
	}
	err := validate(options{dataDir: "./data"})
	if err == nil {
		t.Fatalf("validate() error = nil, want missing s3 env")
	}
	if !strings.Contains(err.Error(), "missing required s3 env") {
		t.Fatalf("validate() error = %v, want missing s3 env", err)
	}
}

func writeFile(t *testing.T, filePath string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
