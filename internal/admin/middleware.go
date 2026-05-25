package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/httpx"
)

// RequireAdmin returns a middleware that 403s any non-admin caller.
// Must be mounted AFTER auth.RequireAuth (so role is set on the context).
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if httpx.UserRoleFrom(c) != "admin" {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "需要管理员权限")
			return
		}
		c.Next()
	}
}
