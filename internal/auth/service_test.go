package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liusx/shadraw/internal/user"
)

// ---- fake repositories ---------------------------------------------------

type fakeUsers struct {
	byID    map[int64]*user.User
	byEmail map[string]int64
	nextID  int64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[int64]*user.User{}, byEmail: map[string]int64{}, nextID: 0}
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*user.User, error) {
	id, ok := f.byEmail[strings.ToLower(email)]
	if !ok {
		return nil, user.ErrNotFound
	}
	return f.copy(f.byID[id]), nil
}

func (f *fakeUsers) FindByID(_ context.Context, id int64) (*user.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return f.copy(u), nil
}

func (f *fakeUsers) Create(_ context.Context, u *user.User) error {
	f.nextID++
	u.ID = f.nextID
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	f.byID[u.ID] = f.copy(u)
	f.byEmail[strings.ToLower(u.Email)] = u.ID
	return nil
}

func (f *fakeUsers) UpdatePassword(_ context.Context, id int64, hash string, mustChange bool) error {
	u, ok := f.byID[id]
	if !ok {
		return user.ErrNotFound
	}
	u.PasswordHash = hash
	u.MustChangePassword = mustChange
	return nil
}

func (f *fakeUsers) EmailExists(_ context.Context, email string) (bool, error) {
	_, ok := f.byEmail[strings.ToLower(email)]
	return ok, nil
}

func (f *fakeUsers) copy(u *user.User) *user.User {
	if u == nil {
		return nil
	}
	c := *u
	return &c
}

type fakeRefresh struct {
	rows   map[int64]*RefreshToken
	byHash map[string]int64
	nextID int64
}

func newFakeRefresh() *fakeRefresh {
	return &fakeRefresh{rows: map[int64]*RefreshToken{}, byHash: map[string]int64{}}
}

func (f *fakeRefresh) Create(_ context.Context, t *RefreshToken) error {
	f.nextID++
	t.ID = f.nextID
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	cp := *t
	f.rows[t.ID] = &cp
	f.byHash[t.TokenHash] = t.ID
	return nil
}

func (f *fakeRefresh) FindByHash(_ context.Context, hash string) (*RefreshToken, error) {
	id, ok := f.byHash[hash]
	if !ok {
		return nil, ErrRefreshNotFound
	}
	cp := *f.rows[id]
	return &cp, nil
}

func (f *fakeRefresh) Revoke(_ context.Context, id int64) error {
	t, ok := f.rows[id]
	if !ok {
		return ErrRefreshNotFound
	}
	t.Revoked = true
	return nil
}

func (f *fakeRefresh) RevokeAllForUser(_ context.Context, userID int64) error {
	for _, t := range f.rows {
		if t.UserID == userID {
			t.Revoked = true
		}
	}
	return nil
}

// ---- test helpers --------------------------------------------------------

func newTestService(t *testing.T) (*Service, *fakeUsers, *fakeRefresh) {
	t.Helper()
	users := newFakeUsers()
	refresh := newFakeRefresh()
	svc := newServiceImpl(users, refresh, "test-secret-of-thirty-two-chars!", time.Now)
	return svc, users, refresh
}

// ---- tests ---------------------------------------------------------------

func TestService_Register_Success(t *testing.T) {
	svc, users, _ := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "alice"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.User.Email != "a@b.com" || resp.User.Role != "user" {
		t.Fatalf("unexpected user: %+v", resp.User)
	}
	if resp.Tokens.AccessToken == "" || resp.Tokens.RefreshToken == "" {
		t.Fatalf("expected tokens, got %+v", resp.Tokens)
	}
	if got := len(users.byID); got != 1 {
		t.Fatalf("expected 1 user, got %d", got)
	}
}

func TestService_Register_DuplicateEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	req := RegisterReq{Email: "dup@x.com", Password: "12345678", DisplayName: "u"}
	if _, err := svc.Register(ctx, req); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := svc.Register(ctx, req)
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestService_Login_WrongPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "rightpass", DisplayName: "u"})

	_, err := svc.Login(ctx, LoginReq{Email: "a@b.com", Password: "wrongpass"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Login_UnknownEmail(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Login(context.Background(), LoginReq{Email: "nobody@x.com", Password: "whatever"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Login_DisabledUser(t *testing.T) {
	svc, users, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterReq{Email: "x@x.com", Password: "12345678", DisplayName: "x"})
	users.byID[1].Disabled = true

	_, err := svc.Login(ctx, LoginReq{Email: "x@x.com", Password: "12345678"})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("want ErrUserDisabled, got %v", err)
	}
}

func TestService_Refresh_RotatesAndRevokes(t *testing.T) {
	svc, _, refresh := newTestService(t)
	ctx := context.Background()
	regResp, _ := svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	oldRaw := regResp.Tokens.RefreshToken
	newPair, err := svc.Refresh(ctx, oldRaw)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newPair.RefreshToken == oldRaw {
		t.Fatal("expected rotated refresh token")
	}
	// old should now be revoked
	oldHash := HashRefreshToken(oldRaw)
	row, _ := refresh.FindByHash(ctx, oldHash)
	if !row.Revoked {
		t.Fatal("old refresh token should be revoked after rotation")
	}
}

func TestService_Refresh_RejectsRevoked(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	regResp, _ := svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	_, _ = svc.Refresh(ctx, regResp.Tokens.RefreshToken) // rotation 1: old becomes revoked
	_, err := svc.Refresh(ctx, regResp.Tokens.RefreshToken)
	if !errors.Is(err, ErrRefreshRevoked) {
		t.Fatalf("want ErrRefreshRevoked, got %v", err)
	}
}

func TestService_Refresh_RejectsExpired(t *testing.T) {
	users := newFakeUsers()
	refresh := newFakeRefresh()
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newServiceImpl(users, refresh, "test-secret-of-thirty-two-chars!", func() time.Time { return past })

	regResp, _ := svc.Register(context.Background(), RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	// Move time well past the refresh TTL.
	svc.now = func() time.Time { return past.Add(RefreshTTL).Add(time.Hour) }
	_, err := svc.Refresh(context.Background(), regResp.Tokens.RefreshToken)
	if !errors.Is(err, ErrRefreshExpired) {
		t.Fatalf("want ErrRefreshExpired, got %v", err)
	}
}

func TestService_ChangePassword_RevokesAllRefresh(t *testing.T) {
	svc, _, refresh := newTestService(t)
	ctx := context.Background()
	resp, _ := svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	err := svc.ChangePassword(ctx, 1, "12345678", "newpassw0rd")
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	row, _ := refresh.FindByHash(ctx, HashRefreshToken(resp.Tokens.RefreshToken))
	if !row.Revoked {
		t.Fatal("refresh token should be revoked after password change")
	}

	// old password no longer works
	if _, err := svc.Login(ctx, LoginReq{Email: "a@b.com", Password: "12345678"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail; got %v", err)
	}
	// new password works
	if _, err := svc.Login(ctx, LoginReq{Email: "a@b.com", Password: "newpassw0rd"}); err != nil {
		t.Fatalf("new password should succeed: %v", err)
	}
}

func TestService_ChangePassword_WrongOldPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	err := svc.ChangePassword(ctx, 1, "wrongold", "newpassw0rd")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Logout_Idempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// unknown token: success without error
	if err := svc.Logout(ctx, "nonexistent"); err != nil {
		t.Fatalf("logout unknown should succeed, got %v", err)
	}
}

func TestService_Logout_Revokes(t *testing.T) {
	svc, _, refresh := newTestService(t)
	ctx := context.Background()
	resp, _ := svc.Register(ctx, RegisterReq{Email: "a@b.com", Password: "12345678", DisplayName: "u"})

	if err := svc.Logout(ctx, resp.Tokens.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	row, _ := refresh.FindByHash(ctx, HashRefreshToken(resp.Tokens.RefreshToken))
	if !row.Revoked {
		t.Fatal("logout should revoke the refresh token")
	}
	// subsequent refresh should fail
	if _, err := svc.Refresh(ctx, resp.Tokens.RefreshToken); !errors.Is(err, ErrRefreshRevoked) {
		t.Fatalf("want ErrRefreshRevoked after logout, got %v", err)
	}
}

func TestService_ResetPasswordByAdmin(t *testing.T) {
	svc, _, refresh := newTestService(t)
	ctx := context.Background()
	resp, _ := svc.Register(ctx, RegisterReq{Email: "u@x.com", Password: "12345678", DisplayName: "u"})

	temp, err := svc.ResetPasswordByAdmin(ctx, 1)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if len(temp) < 8 {
		t.Fatalf("temp pwd too short: %q", temp)
	}

	// old pwd rejected
	if _, err := svc.Login(ctx, LoginReq{Email: "u@x.com", Password: "12345678"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old pwd should fail, got %v", err)
	}
	// new temp pwd works
	if _, err := svc.Login(ctx, LoginReq{Email: "u@x.com", Password: temp}); err != nil {
		t.Fatalf("new pwd should succeed, got %v", err)
	}
	// existing refresh tokens revoked
	row, _ := refresh.FindByHash(ctx, HashRefreshToken(resp.Tokens.RefreshToken))
	if !row.Revoked {
		t.Fatal("admin reset should revoke existing refresh tokens")
	}
}
