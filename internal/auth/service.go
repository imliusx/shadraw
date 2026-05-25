package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/liusx/shadraw/internal/user"
)

// Service-level error sentinels. Handlers map these to HTTP status + error code.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("account disabled")
	ErrRefreshInvalid     = errors.New("refresh token invalid")
	ErrRefreshExpired     = errors.New("refresh token expired")
	ErrRefreshRevoked     = errors.New("refresh token revoked")
)

// userStore is the subset of user.Repository methods the auth service uses.
// Extracted to allow easy mocking in tests.
type userStore interface {
	FindByEmail(ctx context.Context, email string) (*user.User, error)
	FindByID(ctx context.Context, id int64) (*user.User, error)
	Create(ctx context.Context, u *user.User) error
	UpdatePassword(ctx context.Context, id int64, hash string, mustChange bool) error
	EmailExists(ctx context.Context, email string) (bool, error)
}

// refreshStore is the subset of RefreshRepository the auth service uses.
type refreshStore interface {
	Create(ctx context.Context, t *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id int64) error
	RevokeAllForUser(ctx context.Context, userID int64) error
}

// Service holds Auth business logic. Handlers must not bypass it to touch repositories.
type Service struct {
	users     userStore
	refresh   refreshStore
	jwtSecret []byte
	now       func() time.Time
}

// NewService wires the auth service. `now` is injectable so tests can freeze time.
func NewService(users *user.Repository, refresh *RefreshRepository, jwtSecret string, now func() time.Time) *Service {
	return newServiceImpl(users, refresh, jwtSecret, now)
}

func newServiceImpl(users userStore, refresh refreshStore, jwtSecret string, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		users:     users,
		refresh:   refresh,
		jwtSecret: []byte(jwtSecret),
		now:       now,
	}
}

// Register creates a new user account and issues a token pair.
func (s *Service) Register(ctx context.Context, req RegisterReq) (*AuthResponse, error) {
	exists, err := s.users.EmailExists(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailTaken
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	u := &user.User{
		Email:        req.Email,
		PasswordHash: hash,
		DisplayName:  req.DisplayName,
		Role:         user.RoleUser,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return s.issueAuthResponse(ctx, u)
}

// Login validates credentials and issues a token pair.
func (s *Service) Login(ctx context.Context, req LoginReq) (*AuthResponse, error) {
	u, err := s.users.FindByEmail(ctx, req.Email)
	if errors.Is(err, user.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, ErrUserDisabled
	}
	if err := VerifyPassword(u.PasswordHash, req.Password); err != nil {
		if errors.Is(err, ErrPasswordMismatch) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return s.issueAuthResponse(ctx, u)
}

// Refresh rotates the refresh token (issues a new pair, revokes the old one).
func (s *Service) Refresh(ctx context.Context, raw string) (*TokenPair, error) {
	hash := HashRefreshToken(raw)
	row, err := s.refresh.FindByHash(ctx, hash)
	if errors.Is(err, ErrRefreshNotFound) {
		return nil, ErrRefreshInvalid
	}
	if err != nil {
		return nil, err
	}
	if row.Revoked {
		return nil, ErrRefreshRevoked
	}
	if s.now().After(row.ExpiresAt) {
		return nil, ErrRefreshExpired
	}
	u, err := s.users.FindByID(ctx, row.UserID)
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, ErrUserDisabled
	}
	// Rotate: revoke the old, mint a fresh pair.
	if err := s.refresh.Revoke(ctx, row.ID); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, u)
}

// Logout revokes a refresh token. Idempotent: unknown tokens silently succeed.
func (s *Service) Logout(ctx context.Context, raw string) error {
	hash := HashRefreshToken(raw)
	row, err := s.refresh.FindByHash(ctx, hash)
	if errors.Is(err, ErrRefreshNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.refresh.Revoke(ctx, row.ID)
}

// Me fetches the current user by id.
func (s *Service) Me(ctx context.Context, userID int64) (*user.User, error) {
	return s.users.FindByID(ctx, userID)
}

// ChangePassword verifies the old password and sets a new one. All refresh
// tokens for the user are revoked.
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPw, newPw string) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := VerifyPassword(u.PasswordHash, oldPw); err != nil {
		if errors.Is(err, ErrPasswordMismatch) {
			return ErrInvalidCredentials
		}
		return err
	}
	hash, err := HashPassword(newPw)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, userID, hash, false); err != nil {
		return err
	}
	return s.refresh.RevokeAllForUser(ctx, userID)
}

// ResetPasswordByAdmin generates a random temp password, writes it (forcing
// must_change_password=true) and revokes all refresh tokens.
func (s *Service) ResetPasswordByAdmin(ctx context.Context, userID int64) (string, error) {
	if _, err := s.users.FindByID(ctx, userID); err != nil {
		return "", err
	}
	temp, err := randomTempPassword()
	if err != nil {
		return "", err
	}
	hash, err := HashPassword(temp)
	if err != nil {
		return "", err
	}
	if err := s.users.UpdatePassword(ctx, userID, hash, true); err != nil {
		return "", err
	}
	if err := s.refresh.RevokeAllForUser(ctx, userID); err != nil {
		return "", err
	}
	return temp, nil
}

func randomTempPassword() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// issueAuthResponse mints tokens AND packages the public user payload.
func (s *Service) issueAuthResponse(ctx context.Context, u *user.User) (*AuthResponse, error) {
	tokens, err := s.issueTokens(ctx, u)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{User: ToUserDTO(u), Tokens: *tokens}, nil
}

func (s *Service) issueTokens(ctx context.Context, u *user.User) (*TokenPair, error) {
	now := s.now()
	access, err := SignAccessToken(s.jwtSecret, u.ID, string(u.Role), now)
	if err != nil {
		return nil, err
	}
	rawRefresh, hash, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	rt := &RefreshToken{
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(RefreshTTL),
	}
	if err := s.refresh.Create(ctx, rt); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(AccessTTL.Seconds()),
	}, nil
}

// ToUserDTO converts a domain user to its public DTO.
func ToUserDTO(u *user.User) UserDTO {
	return UserDTO{
		ID:                 strconv.FormatInt(u.ID, 10),
		Email:              u.Email,
		DisplayName:        u.DisplayName,
		Role:               string(u.Role),
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
