package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// RefreshToken mirrors refresh_tokens. The plaintext token is never persisted;
// only sha256(token) lives in TokenHash.
type RefreshToken struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	UserID    int64     `gorm:"column:user_id"`
	TokenHash string    `gorm:"column:token_hash"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
	Revoked   bool      `gorm:"column:revoked"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// ErrRefreshNotFound is returned when the token cannot be located.
var ErrRefreshNotFound = errors.New("refresh token not found")

// RefreshRepository persists refresh tokens.
type RefreshRepository struct {
	db *gorm.DB
}

func NewRefreshRepository(db *gorm.DB) *RefreshRepository {
	return &RefreshRepository{db: db}
}

// Create persists a new refresh token row.
func (r *RefreshRepository) Create(ctx context.Context, t *RefreshToken) error {
	return r.db.WithContext(ctx).
		Select("user_id", "token_hash", "expires_at", "revoked").
		Create(t).Error
}

// FindByHash returns the (non-revoked, non-expired) token row matching hash.
func (r *RefreshRepository) FindByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	var t RefreshToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ?", hash).
		Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRefreshNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Revoke marks a single token as revoked.
func (r *RefreshRepository) Revoke(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&RefreshToken{}).
		Where("id = ?", id).
		Update("revoked", true).Error
}

// RevokeAllForUser revokes every active token for a user (used on password change).
func (r *RefreshRepository) RevokeAllForUser(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Model(&RefreshToken{}).
		Where("user_id = ? AND revoked = false", userID).
		Update("revoked", true).Error
}
