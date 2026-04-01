package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

type RateLimiter struct {
	limit   int
	window  time.Duration
	mu      sync.Mutex
	entries map[string]rateLimitEntry
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]rateLimitEntry),
	}
}

func (l *RateLimiter) Middleware(bucket string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := clientIdentifier(r)
			now := time.Now()
			key := bucket + ":" + clientID

			allowed, remaining, resetIn := l.take(key, now)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(resetIn.Seconds())+1))
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (l *RateLimiter) take(key string, now time.Time) (allowed bool, remaining int, resetIn time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for entryKey, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			delete(l.entries, entryKey)
		}
	}

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = rateLimitEntry{
			windowStart: now,
			count:       1,
		}
		return true, l.limit - 1, l.window
	}

	if entry.count >= l.limit {
		resetIn = l.window - now.Sub(entry.windowStart)
		if resetIn < 0 {
			resetIn = 0
		}
		return false, 0, resetIn
	}

	entry.count++
	l.entries[key] = entry
	return true, l.limit - entry.count, l.window - now.Sub(entry.windowStart)
}

func clientIdentifier(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return fmt.Sprintf("unknown:%s", r.UserAgent())
}
