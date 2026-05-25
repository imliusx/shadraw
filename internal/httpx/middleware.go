package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ctxKey string

const (
	ctxRequestID ctxKey = "requestID"
	ctxUserID    ctxKey = "userID"
	ctxUserRole  ctxKey = "userRole"
)

// RequestID assigns each request a hex id, exposed via X-Request-ID and slog.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set(string(ctxRequestID), id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Next()
	}
}

// Logger logs each request with method/path/status/latency.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latencyMs", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"requestID", c.GetString(string(ctxRequestID)),
		}
		if uid := c.GetString(string(ctxUserID)); uid != "" {
			attrs = append(attrs, "userID", uid)
		}
		switch {
		case c.Writer.Status() >= 500:
			slog.Error("http", attrs...)
		case c.Writer.Status() >= 400:
			slog.Warn("http", attrs...)
		default:
			slog.Info("http", attrs...)
		}
	}
}

// Recovery converts panics to 500 internal_error.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic", "err", recovered, "path", c.Request.URL.Path)
		Fail(c, http.StatusInternalServerError, CodeInternalError, "internal error")
	})
}

// SecurityHeaders sets safe defaults.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// RequestIDFrom extracts the request id from gin.Context (or empty).
func RequestIDFrom(c *gin.Context) string {
	return c.GetString(string(ctxRequestID))
}

// SetAuth stores the authenticated user info into the request context for downstream handlers.
func SetAuth(c *gin.Context, userID, role string) {
	c.Set(string(ctxUserID), userID)
	c.Set(string(ctxUserRole), role)
}

// UserIDFrom returns the authenticated user id (or empty if not authenticated).
func UserIDFrom(c *gin.Context) string { return c.GetString(string(ctxUserID)) }

// UserRoleFrom returns the authenticated user role (or empty if not authenticated).
func UserRoleFrom(c *gin.Context) string { return c.GetString(string(ctxUserRole)) }

// WithAuth returns a context carrying user id/role (for non-gin call sites).
func WithAuth(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, ctxUserID, userID)
	ctx = context.WithValue(ctx, ctxUserRole, role)
	return ctx
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}
