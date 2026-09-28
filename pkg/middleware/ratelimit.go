package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

// clientBucket tracks token bucket rate limiting state for a single client IP
type clientBucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimiter manages in-memory token bucket rate limiting per client IP
type RateLimiter struct {
	mu           sync.Mutex
	rate         float64 // tokens per second
	burst        int     // maximum burst capacity
	clients      map[string]*clientBucket
	cleanupTimer *time.Ticker
	stopCleanup  chan struct{}
}

// NewRateLimiter creates a new RateLimiter with specified requests per second and burst size
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:         rps,
		burst:        burst,
		clients:      make(map[string]*clientBucket),
		cleanupTimer: time.NewTicker(3 * time.Minute),
		stopCleanup:  make(chan struct{}),
	}

	// Janitor routine to purge stale client entries and prevent memory leaks
	go rl.cleanupLoop()

	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTimer.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for ip, bucket := range rl.clients {
				if bucket.lastRefill.Before(cutoff) {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCleanup:
			rl.cleanupTimer.Stop()
			return
		}
	}
}

// Stop stops the background cleanup worker
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// Allow checks if the client IP is allowed to make a request and consumes 1 token if available
func (rl *RateLimiter) Allow(ip string) (allowed bool, remaining int, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.clients[ip]
	if !exists {
		bucket = &clientBucket{
			tokens:     float64(rl.burst),
			lastRefill: now,
		}
		rl.clients[ip] = bucket
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * rl.rate
	if bucket.tokens > float64(rl.burst) {
		bucket.tokens = float64(rl.burst)
	}
	bucket.lastRefill = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, int(bucket.tokens), 0
	}

	// Calculate wait time until at least 1 token is available
	missing := 1.0 - bucket.tokens
	retrySeconds := missing / rl.rate
	return false, 0, time.Duration(retrySeconds*float64(time.Second)) + (100 * time.Millisecond)
}

// extractClientIP retrieves the real client IP considering proxy headers
func extractClientIP(r *http.Request) string {
	// 1. Check X-Forwarded-For (leftmost is client IP)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	// 2. Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if ip != "" {
			return ip
		}
	}

	// 3. Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// RateLimit returns an HTTP middleware using the provided RateLimiter
func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r)

			allowed, remaining, retryAfter := rl.Allow(ip)

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.burst))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

			if !allowed {
				retryAfterSecs := int(retryAfter.Seconds())
				if retryAfterSecs < 1 {
					retryAfterSecs = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSecs))

				response.Error(
					w,
					http.StatusTooManyRequests,
					response.ErrCodeRateLimited,
					"Rate limit exceeded. Please wait before making more requests.",
					map[string]any{
						"retry_after_seconds": retryAfterSecs,
						"limit":               rl.burst,
					},
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
