// Package main migrates local image blobs into the configured S3-compatible
// store while preserving the relative object keys stored in records.image_path.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/liusx/shadraw/internal/blob"
)

type options struct {
	envFile      string
	dataDir      string
	endpoint     string
	region       string
	bucket       string
	accessKey    string
	secretKey    string
	usePathStyle bool
	dryRun       bool
	verifyOnly   bool
}

type stats struct {
	scanned  int
	uploaded int
	verified int
	skipped  int
	failed   int
	bytes    int64
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate blobs: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := flag.String("env-file", ".env", "optional dotenv file to load before reading env")
	dataDir := flag.String("data-dir", "", "local DATA_DIR containing images/")
	endpoint := flag.String("endpoint", "", "S3 endpoint, e.g. http://localhost:9000")
	region := flag.String("region", "", "S3 region")
	bucket := flag.String("bucket", "", "S3 bucket")
	accessKey := flag.String("access-key", "", "S3 access key")
	secretKey := flag.String("secret-key", "", "S3 secret key")
	usePathStyle := flag.Bool("path-style", true, "use S3 path-style addressing")
	dryRun := flag.Bool("dry-run", false, "print files that would be uploaded without writing to S3")
	verifyOnly := flag.Bool("verify-only", false, "verify S3 objects against local files without uploading")
	flag.Parse()

	if *envFile != "" {
		if err := loadDotenv(*envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	opts := options{
		envFile:      *envFile,
		dataDir:      firstNonEmpty(*dataDir, getenv("DATA_DIR", "./data")),
		endpoint:     firstNonEmpty(*endpoint, os.Getenv("S3_ENDPOINT")),
		region:       firstNonEmpty(*region, getenv("S3_REGION", "us-east-1")),
		bucket:       firstNonEmpty(*bucket, os.Getenv("S3_BUCKET")),
		accessKey:    firstNonEmpty(*accessKey, os.Getenv("S3_ACCESS_KEY_ID")),
		secretKey:    firstNonEmpty(*secretKey, os.Getenv("S3_SECRET_ACCESS_KEY")),
		usePathStyle: *usePathStyle,
		dryRun:       *dryRun,
		verifyOnly:   *verifyOnly,
	}
	if opts.dryRun && opts.verifyOnly {
		return errors.New("-dry-run and -verify-only cannot be used together")
	}
	if raw, ok := os.LookupEnv("S3_USE_PATH_STYLE"); ok && *usePathStyle {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			opts.usePathStyle = parsed
		}
	}
	if err := validate(opts); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var store blob.Store
	if !opts.dryRun {
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		s3Store, err := blob.NewS3(initCtx, blob.S3Config{
			Endpoint:     opts.endpoint,
			Region:       opts.region,
			Bucket:       opts.bucket,
			AccessKey:    opts.accessKey,
			SecretKey:    opts.secretKey,
			UsePathStyle: opts.usePathStyle,
		})
		if err != nil {
			return err
		}
		store = s3Store
	}

	result, err := migrate(ctx, opts, store)
	if err != nil {
		return err
	}
	fmt.Printf("scanned=%d uploaded=%d verified=%d skipped=%d failed=%d bytes=%d\n",
		result.scanned, result.uploaded, result.verified, result.skipped, result.failed, result.bytes)
	if result.failed > 0 {
		return fmt.Errorf("%d file(s) failed", result.failed)
	}
	return nil
}

func validate(opts options) error {
	if opts.dataDir == "" {
		return errors.New("DATA_DIR is required")
	}
	if opts.dryRun {
		return nil
	}
	var missing []string
	for key, value := range map[string]string{
		"S3_ENDPOINT":          opts.endpoint,
		"S3_BUCKET":            opts.bucket,
		"S3_ACCESS_KEY_ID":     opts.accessKey,
		"S3_SECRET_ACCESS_KEY": opts.secretKey,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required s3 env: %s", strings.Join(missing, ", "))
	}
	return nil
}

func migrate(ctx context.Context, opts options, store blob.Store) (stats, error) {
	var result stats
	imagesDir := filepath.Join(opts.dataDir, "images")
	if _, err := os.Stat(imagesDir); err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("images directory does not exist: %s", imagesDir)
		}
		return result, fmt.Errorf("stat images directory: %w", err)
	}

	err := filepath.WalkDir(imagesDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "skip unreadable path %s: %v\n", filePath, walkErr)
			return nil
		}
		if entry.IsDir() {
			if isHidden(entry.Name()) && filePath != imagesDir {
				result.skipped++
				return filepath.SkipDir
			}
			return nil
		}
		result.scanned++
		if isHidden(entry.Name()) {
			result.skipped++
			return nil
		}

		rel, err := filepath.Rel(opts.dataDir, filePath)
		if err != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "rel path failed %s: %v\n", filePath, err)
			return nil
		}
		key := filepath.ToSlash(rel)
		bucketSegment, userKey, fileKey, err := splitObjectKey(key)
		if err != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "invalid object key %s: %v\n", key, err)
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "stat file failed %s: %v\n", filePath, err)
			return nil
		}
		result.bytes += info.Size()

		if opts.dryRun {
			result.uploaded++
			fmt.Printf("would upload %s\n", key)
			return nil
		}

		if opts.verifyOnly {
			size, err := store.Stat(ctx, key)
			if err != nil {
				result.failed++
				fmt.Fprintf(os.Stderr, "verify failed %s: %v\n", key, err)
				return nil
			}
			if size != info.Size() {
				result.failed++
				fmt.Fprintf(os.Stderr, "verify size mismatch %s: local=%d remote=%d\n", key, info.Size(), size)
				return nil
			}
			result.verified++
			fmt.Printf("verified %s\n", key)
			return nil
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "read file failed %s: %v\n", filePath, err)
			return nil
		}
		if _, err := store.Put(ctx, bucketSegment, userKey, fileKey, data); err != nil {
			result.failed++
			fmt.Fprintf(os.Stderr, "upload failed %s: %v\n", key, err)
			return nil
		}
		result.uploaded++
		fmt.Printf("uploaded %s\n", key)
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func splitObjectKey(key string) (string, string, string, error) {
	parts := strings.Split(key, "/")
	if len(parts) < 3 || parts[0] != "images" || parts[1] == "" {
		return "", "", "", fmt.Errorf("expected images/<user>/<file>, got %q", key)
	}
	fileKey := path.Join(parts[2:]...)
	if fileKey == "." || fileKey == "" {
		return "", "", "", fmt.Errorf("missing file name in %q", key)
	}
	return parts[0], parts[1], fileKey, nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func loadDotenv(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=value", filePath, lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", filePath, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
