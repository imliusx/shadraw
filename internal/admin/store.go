// Package admin owns upstream-config, runtime, user management and stats
// endpoints. All routes must be mounted behind RequireAdmin.
package admin

import (
	"context"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/liusx/shadraw/internal/crypto"
	"github.com/liusx/shadraw/internal/upstream"
)

// UpstreamConfig models the single-row upstream_configs table.
type UpstreamConfig struct {
	ID                int16     `gorm:"column:id;primaryKey"`
	BaseURL           string    `gorm:"column:base_url"`
	APIKeyCipher      []byte    `gorm:"column:api_key_cipher"`
	EnabledModels     JSONArray `gorm:"column:enabled_models;type:jsonb"`
	WorkerConcurrency int16     `gorm:"column:worker_concurrency"`
	UpdatedBy         *int64    `gorm:"column:updated_by"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (UpstreamConfig) TableName() string { return "upstream_configs" }

// Store provides access + in-memory caching of the current upstream config.
// Worker pool reads from Snapshot(); admin write updates DB + cache atomically.
type Store struct {
	db     *gorm.DB
	cipher *crypto.AESGCM

	mu      sync.RWMutex
	cached  UpstreamConfig
	apiKey  string // decrypted, only kept in memory
	loaded  bool
	resizer func(int)
}

func NewStore(db *gorm.DB, cipher *crypto.AESGCM) *Store {
	return &Store{db: db, cipher: cipher}
}

// SetResizer wires the worker pool's Resize() so admin runtime changes apply hot.
func (s *Store) SetResizer(fn func(int)) {
	s.mu.Lock()
	s.resizer = fn
	s.mu.Unlock()
}

// Load reads the single row from DB and decrypts the API key.
func (s *Store) Load(ctx context.Context) error {
	var row UpstreamConfig
	err := s.db.WithContext(ctx).
		Select("id", "base_url", "api_key_cipher", "enabled_models", "worker_concurrency", "updated_by", "created_at", "updated_at").
		Where("id = 1").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Seed empty row so admin can fill it in.
		row = UpstreamConfig{ID: 1, EnabledModels: JSONArray{}, WorkerConcurrency: 4}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var apiKey string
	if len(row.APIKeyCipher) > 0 {
		plain, derr := s.cipher.Decrypt(row.APIKeyCipher)
		if derr != nil {
			return derr
		}
		apiKey = string(plain)
	}

	s.mu.Lock()
	s.cached = row
	s.apiKey = apiKey
	s.loaded = true
	s.mu.Unlock()
	return nil
}

// Snapshot returns the live upstream credentials (used by worker pool).
func (s *Store) Snapshot() upstream.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return upstream.Config{BaseURL: s.cached.BaseURL, APIKey: s.apiKey}
}

// EnabledModels returns the admin-curated model whitelist.
func (s *Store) EnabledModels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.cached.EnabledModels))
	copy(out, s.cached.EnabledModels)
	return out
}

// WorkerConcurrency returns the live concurrency value.
func (s *Store) WorkerConcurrency() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int(s.cached.WorkerConcurrency)
}

// UpdateUpstreamArgs carries the editable fields.
type UpdateUpstreamArgs struct {
	BaseURL       string
	APIKey        *string // nil = leave unchanged; "" = clear
	EnabledModels []string
	ActorID       int64
}

// UpdateUpstream writes config to DB + cache.
func (s *Store) UpdateUpstream(ctx context.Context, a UpdateUpstreamArgs) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cipher := s.cached.APIKeyCipher
	apiKey := s.apiKey
	if a.APIKey != nil {
		if *a.APIKey == "" {
			cipher = nil
			apiKey = ""
		} else {
			blob, err := s.cipher.Encrypt([]byte(*a.APIKey))
			if err != nil {
				return err
			}
			cipher = blob
			apiKey = *a.APIKey
		}
	}

	updates := map[string]any{
		"base_url":       a.BaseURL,
		"enabled_models": JSONArray(a.EnabledModels),
		"api_key_cipher": cipher,
		"updated_by":     a.ActorID,
	}
	if err := s.db.WithContext(ctx).Model(&UpstreamConfig{}).
		Where("id = 1").
		Updates(updates).Error; err != nil {
		return err
	}

	s.cached.BaseURL = a.BaseURL
	s.cached.EnabledModels = JSONArray(a.EnabledModels)
	s.cached.APIKeyCipher = cipher
	s.cached.UpdatedBy = &a.ActorID
	s.apiKey = apiKey
	return nil
}

// UpdateWorkerConcurrency persists + hot-applies the new value.
func (s *Store) UpdateWorkerConcurrency(ctx context.Context, n int, actorID int64) error {
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	if err := s.db.WithContext(ctx).Model(&UpstreamConfig{}).
		Where("id = 1").
		Updates(map[string]any{
			"worker_concurrency": n,
			"updated_by":         actorID,
		}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.cached.WorkerConcurrency = int16(n)
	fn := s.resizer
	s.mu.Unlock()
	if fn != nil {
		fn(n)
	}
	return nil
}

// View returns the public DTO with apiKey masked.
func (s *Store) View() UpstreamConfigDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dto := UpstreamConfigDTO{
		BaseURL:           s.cached.BaseURL,
		EnabledModels:     append([]string(nil), s.cached.EnabledModels...),
		WorkerConcurrency: int(s.cached.WorkerConcurrency),
	}
	if s.apiKey != "" {
		dto.APIKeyMasked = crypto.MaskAPIKey(s.apiKey)
		dto.APIKeySet = true
	}
	return dto
}
