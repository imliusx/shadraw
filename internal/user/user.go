// Package user owns the user model and its repository.
package user

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Role enumerates user roles.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// User mirrors the users table. All fields are declared explicitly per the
// project DB conventions (no gorm.Model).
type User struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	Email              string    `gorm:"column:email"`
	PasswordHash       string    `gorm:"column:password_hash"`
	DisplayName        string    `gorm:"column:display_name"`
	Role               Role      `gorm:"column:role"`
	Disabled           bool      `gorm:"column:disabled"`
	MustChangePassword bool      `gorm:"column:must_change_password"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName forces the table name to "users" instead of GORM's auto-pluralization.
func (User) TableName() string { return "users" }

// ErrNotFound is returned when the lookup misses.
var ErrNotFound = errors.New("user not found")

// Repository is the persistence boundary for users.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// FindByEmail returns the user with the given email (case-insensitive via CITEXT).
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).
		Select("id", "email", "password_hash", "display_name", "role", "disabled", "must_change_password", "created_at", "updated_at").
		Where("email = ?", email).
		Take(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByID returns the user with the given id.
func (r *Repository) FindByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).
		Select("id", "email", "password_hash", "display_name", "role", "disabled", "must_change_password", "created_at", "updated_at").
		Where("id = ?", id).
		Take(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a new user and writes the generated id back to u.
func (r *Repository) Create(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).
		Select("email", "password_hash", "display_name", "role", "disabled", "must_change_password").
		Create(u).Error
}

// UpdatePassword updates the password hash for a user.
func (r *Repository) UpdatePassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password_hash":        hash,
			"must_change_password": mustChange,
		}).Error
}

// SetRole forces a user's role.
func (r *Repository) SetRole(ctx context.Context, id int64, role Role) error {
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Update("role", role).Error
}

// EmailExists reports whether the email is taken.
func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&User{}).
		Where("email = ?", email).
		Count(&n).Error
	return n > 0, err
}
