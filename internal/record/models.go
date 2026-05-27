// Package record owns records / projects models and persistence.
package record

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/liusx/shadraw/internal/imagegen"
)

// Status enumerates record lifecycle.
type Status string

const (
	StatusWaiting   Status = "waiting"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Record mirrors the records table.
type Record struct {
	ID              int64           `gorm:"column:id;primaryKey"`
	UUID            string          `gorm:"column:uuid;type:uuid"`
	UserID          int64           `gorm:"column:user_id"`
	ProjectID       *int64          `gorm:"column:project_id"`
	Prompt          string          `gorm:"column:prompt"`
	Model           string          `gorm:"column:model"`
	ImageParams     imagegen.Params `gorm:"column:image_params;type:jsonb"`
	Status          Status          `gorm:"column:status"`
	Favorite        bool            `gorm:"column:favorite"`
	IsPublic        bool            `gorm:"column:is_public"`
	PromptPublic    bool            `gorm:"column:prompt_public"`
	ImagePath       *string         `gorm:"column:image_path"`
	Error           *string         `gorm:"column:error"`
	UpstreamError   *string         `gorm:"column:upstream_error"`
	ReferenceImages StringSlice     `gorm:"column:reference_images;type:jsonb"`
	StartedAt       *time.Time      `gorm:"column:started_at"`
	CompletedAt     *time.Time      `gorm:"column:completed_at"`
	PublishedAt     *time.Time      `gorm:"column:published_at"`
	CreatedAt       time.Time       `gorm:"column:created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at"`
}

func (Record) TableName() string { return "records" }

// RecordFavorite stores a user's collection relation for a record.
type RecordFavorite struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	UserID    int64     `gorm:"column:user_id"`
	RecordID  int64     `gorm:"column:record_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (RecordFavorite) TableName() string { return "record_favorites" }

// Project mirrors the projects table.
type Project struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	UserID    int64     `gorm:"column:user_id"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Project) TableName() string { return "projects" }

// StringSlice is a GORM adapter so a Go []string maps to a Postgres JSONB column.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal([]string(s))
}

func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("StringSlice.Scan: unsupported type")
	}
	return json.Unmarshal(b, s)
}
