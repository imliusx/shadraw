package httpx

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// KeyFunc selects the rate-limit key for a request. Common choices: KeyByIP,
// KeyByUser.
type KeyFunc func(c *gin.Context) string

// KeyByIP returns the client IP as the rate-limit key.
func KeyByIP(c *gin.Context) string { return "ip:" + c.ClientIP() }

// KeyByUser returns the authenticated user id; falls back to IP if anonymous.
func KeyByUser(c *gin.Context) string {
	if uid := UserIDFrom(c); uid != "" {
		return "user:" + uid
	}
	return KeyByIP(c)
}

// RateLimit returns middleware that enforces `limit` requests per `window`
// against the key returned by keyFn. Uses an in-memory fixed window with
// lazy eviction — fine for a single-instance MVP.
func RateLimit(limit int, window time.Duration, keyFn KeyFunc) gin.HandlerFunc {
	l := newLimiter(limit, window)
	return func(c *gin.Context) {
		key := keyFn(c)
		ok, retryAfter := l.allow(key)
		if !ok {
			c.Writer.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			Fail(c, http.StatusTooManyRequests, CodeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}

type window struct {
	resetAt time.Time
	count   int
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
}

func newLimiter(limit int, win time.Duration) *limiter {
	return &limiter{limit: limit, window: win, hits: make(map[string]*window)}
}

func (l *limiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w, ok := l.hits[key]
	if !ok || now.After(w.resetAt) {
		l.hits[key] = &window{resetAt: now.Add(l.window), count: 1}
		l.gc(now)
		return true, 0
	}
	if w.count >= l.limit {
		return false, time.Until(w.resetAt)
	}
	w.count++
	return true, 0
}

// gc removes expired entries opportunistically — runs at most once per call.
func (l *limiter) gc(now time.Time) {
	if len(l.hits) < 1024 {
		return
	}
	for k, v := range l.hits {
		if now.After(v.resetAt) {
			delete(l.hits, k)
		}
	}
}
