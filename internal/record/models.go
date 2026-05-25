// Package record owns records / projects models and persistence.
package record

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
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
	ID              int64        `gorm:"column:id;primaryKey"`
	UUID            string       `gorm:"column:uuid;type:uuid"`
	UserID          int64        `gorm:"column:user_id"`
	ProjectID       *int64       `gorm:"column:project_id"`
	Prompt          string       `gorm:"column:prompt"`
	Model           string       `gorm:"column:model"`
	Ratio           string       `gorm:"column:ratio"`
	Pixels          string       `gorm:"column:pixels"`
	Status          Status       `gorm:"column:status"`
	Favorite        bool         `gorm:"column:favorite"`
	ImagePath       *string      `gorm:"column:image_path"`
	Error           *string      `gorm:"column:error"`
	ReferenceImages StringSlice  `gorm:"column:reference_images;type:jsonb"`
	StartedAt       *time.Time   `gorm:"column:started_at"`
	CompletedAt     *time.Time   `gorm:"column:completed_at"`
	CreatedAt       time.Time    `gorm:"column:created_at"`
	UpdatedAt       time.Time    `gorm:"column:updated_at"`
}

func (Record) TableName() string { return "records" }

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
