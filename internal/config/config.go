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
	Port        int
	LogLevel    string
	DBDSN       string
	JWTSecret   string
	AdminEmail  string
	MasterKey   string
	CORSOrigins []string
	DataDir     string
}

// Load reads env and returns a populated Config. Missing required values
// produce an error; callers should panic with the error so misconfig is
// caught at boot.
func Load() (*Config, error) {
	cfg := &Config{
		Port:     getEnvInt("PORT", 8080),
		LogLevel: getEnv("LOG_LEVEL", "info"),
		DataDir:  getEnv("DATA_DIR", "./data"),
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
