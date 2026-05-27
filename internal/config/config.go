// Package config loads runtime configuration from environment variables.
// Required keys panic at boot to fail fast.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           int
	LogLevel       string
	DBDSN          string
	JWTSecret      string
	AdminEmail     string
	MasterKey      string
	CORSOrigins    []string
	DataDir        string
	BlobDriver     string
	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
}

// Load reads env and returns a populated Config. Missing required values
// produce an error; callers should panic with the error so misconfig is
// caught at boot.
func Load() (*Config, error) {
	cfg := &Config{
		Port:           getEnvInt("PORT", 8080),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		DataDir:        getEnv("DATA_DIR", "./data"),
		BlobDriver:     strings.ToLower(getEnv("BLOB_DRIVER", "local")),
		S3Endpoint:     strings.TrimSpace(getEnv("S3_ENDPOINT", "")),
		S3Region:       getEnv("S3_REGION", "us-east-1"),
		S3Bucket:       strings.TrimSpace(getEnv("S3_BUCKET", "")),
		S3AccessKey:    strings.TrimSpace(getEnv("S3_ACCESS_KEY_ID", "")),
		S3SecretKey:    strings.TrimSpace(getEnv("S3_SECRET_ACCESS_KEY", "")),
		S3UsePathStyle: getEnvBool("S3_USE_PATH_STYLE", true),
	}

	required := map[string]*string{
		"DB_DSN":      &cfg.DBDSN,
		"JWT_SECRET":  &cfg.JWTSecret,
		"ADMIN_EMAIL": &cfg.AdminEmail,
		"MASTER_KEY":  &cfg.MasterKey,
	}
	var missing []string
	for key, dest := range required {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			missing = append(missing, key)
			continue
		}
		*dest = v
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if cfg.BlobDriver != "local" && cfg.BlobDriver != "s3" {
		return nil, fmt.Errorf("BLOB_DRIVER must be local or s3")
	}
	if cfg.BlobDriver == "s3" {
		var s3Missing []string
		for key, value := range map[string]string{
			"S3_ENDPOINT":          cfg.S3Endpoint,
			"S3_BUCKET":            cfg.S3Bucket,
			"S3_ACCESS_KEY_ID":     cfg.S3AccessKey,
			"S3_SECRET_ACCESS_KEY": cfg.S3SecretKey,
		} {
			if value == "" {
				s3Missing = append(s3Missing, key)
			}
		}
		if len(s3Missing) > 0 {
			return nil, fmt.Errorf("missing required s3 env: %s", strings.Join(s3Missing, ", "))
		}
	}

	origins := getEnv("CORS_ORIGINS", "http://localhost:3000")
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
