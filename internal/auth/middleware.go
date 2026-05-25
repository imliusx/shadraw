package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/user"
)

// UserFinder is the small subset of user.Repository needed by RequireAuth.
// Accepting an interface makes the middleware testable with fakes.
type UserFinder interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// RequireAuth validates the Authorization bearer token and stores user info in ctx.
func RequireAuth(jwtSecret string, users UserFinder) gin.HandlerFunc {
	secret := []byte(jwtSecret)
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "缺少 access token")
			return
		}
		claims, err := ParseAccessToken(secret, token)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "access token 无效或已过期")
			return
		}
		if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "access token 已过期")
			return
		}
		u, err := users.FindByID(c.Request.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "用户不存在")
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
			return
		}
		if u.Disabled {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeAccountDisabled, "账号已禁用")
			return
		}
		httpx.SetAuth(c, strconv.FormatInt(u.ID, 10), string(u.Role))
		c.Next()
	}
}

func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
