// Package auth implements registration, login, JWT issuance, refresh-token
// rotation and the password lifecycle.
package auth

// RegisterReq is the body for POST /api/v1/auth/register.
type RegisterReq struct {
	Email       string `json:"email" binding:"required,email,max=254"`
	Password    string `json:"password" binding:"required,min=8,max=128"`
	DisplayName string `json:"displayName" binding:"required,min=1,max=32"`
}

// LoginReq is the body for POST /api/v1/auth/login.
type LoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshReq is the body for POST /api/v1/auth/refresh.
type RefreshReq struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// LogoutReq is the body for POST /api/v1/auth/logout.
type LogoutReq struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// ChangePasswordReq is the body for POST /api/v1/auth/password.
type ChangePasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=128"`
}

// UserDTO is the canonical user shape returned to clients. id is a string
// (see API spec §10.5) to avoid JS Number precision loss.
type UserDTO struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"mustChangePassword"`
	CreatedAt          string `json:"createdAt"`
}

// TokenPair carries the two tokens returned by login/register/refresh.
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"` // seconds until accessToken expires
}

// AuthResponse is returned by register/login.
type AuthResponse struct {
	User   UserDTO   `json:"user"`
	Tokens TokenPair `json:"tokens"`
}
