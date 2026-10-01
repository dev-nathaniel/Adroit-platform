package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tier represents a customer subscription or access tier
type Tier string

const (
	TierFree       Tier = "free"
	TierPro        Tier = "pro"
	TierEnterprise Tier = "enterprise"
	TierInternal   Tier = "internal"
)

// TierQuota defines quota limits for a tier
type TierQuota struct {
	Tier        Tier          `json:"tier"`
	Rate        float64       `json:"rate"`  // Tokens per second
	Burst       int64         `json:"burst"` // Burst capacity / limit
	Window      time.Duration `json:"window"`
	Description string        `json:"description"`
}

// DefaultTierQuotas provides production-grade tier allocations
var DefaultTierQuotas = map[Tier]TierQuota{
	TierFree: {
		Tier:        TierFree,
		Rate:        100.0 / 60.0, // 100 requests per minute
		Burst:       30,
		Window:      1 * time.Minute,
		Description: "Free tier (100 req/min, burst 30)",
	},
	TierPro: {
		Tier:        TierPro,
		Rate:        5000.0 / 60.0, // 5,000 requests per minute
		Burst:       250,
		Window:      1 * time.Minute,
		Description: "Pro tier (5,000 req/min, burst 250)",
	},
	TierEnterprise: {
		Tier:        TierEnterprise,
		Rate:        50000.0 / 60.0, // 50,000 requests per minute
		Burst:       2000,
		Window:      1 * time.Minute,
		Description: "Enterprise tier (50,000 req/min, burst 2,000)",
	},
	TierInternal: {
		Tier:        TierInternal,
		Rate:        100000.0 / 60.0, // 100,000 requests per minute
		Burst:       5000,
		Window:      1 * time.Minute,
		Description: "Internal platform tier (100,000 req/min)",
	},
}

// TierLimiter manages separate rate limiters per customer tier
type TierLimiter struct {
	baseCfg  Config
	quotas   map[Tier]TierQuota
	limiters map[Tier]Limiter
	mu       sync.RWMutex
	closed   bool
}

// NewTierLimiter creates a tier-aware rate limiter
func NewTierLimiter(baseCfg Config, customQuotas map[Tier]TierQuota) (*TierLimiter, error) {
	quotas := make(map[Tier]TierQuota)
	for k, v := range DefaultTierQuotas {
		quotas[k] = v
	}
	for k, v := range customQuotas {
		quotas[k] = v
	}

	tl := &TierLimiter{
		baseCfg:  baseCfg,
		quotas:   quotas,
		limiters: make(map[Tier]Limiter),
	}

	// Pre-initialize limiters for standard tiers
	for tier, q := range quotas {
		tierCfg := baseCfg
		tierCfg.Rate = q.Rate
		tierCfg.Burst = q.Burst
		tierCfg.Window = q.Window
		tierCfg.KeyPrefix = fmt.Sprintf("%stier:%s:", baseCfg.KeyPrefix, tier)

		lim, err := New(tierCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize limiter for tier %s: %w", tier, err)
		}
		tl.limiters[tier] = lim
	}

	return tl, nil
}

// getLimiter returns the Limiter for the given tier, falling back to Free
func (tl *TierLimiter) getLimiter(tier Tier) (Limiter, Tier) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	if lim, exists := tl.limiters[tier]; exists {
		return lim, tier
	}
	// Fallback to Free tier
	return tl.limiters[TierFree], TierFree
}

// Allow checks if 1 request is permitted for the given tier and key
func (tl *TierLimiter) Allow(ctx context.Context, tier Tier, key string) (*Result, error) {
	return tl.AllowN(ctx, tier, key, 1)
}

// AllowN checks if n requests are permitted for the given tier and key
func (tl *TierLimiter) AllowN(ctx context.Context, tier Tier, key string, n int64) (*Result, error) {
	tl.mu.RLock()
	if tl.closed {
		tl.mu.RUnlock()
		return nil, ErrLimiterClosed
	}
	tl.mu.RUnlock()

	lim, _ := tl.getLimiter(tier)
	if lim == nil {
		return nil, fmt.Errorf("%w: no limiter configured for tier %s", ErrInvalidConfig, tier)
	}

	return lim.AllowN(ctx, key, n)
}

// Reset clears rate limit counters for a specific tier and key
func (tl *TierLimiter) Reset(ctx context.Context, tier Tier, key string) error {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	if lim, exists := tl.limiters[tier]; exists {
		return lim.Reset(ctx, key)
	}
	return nil
}

// Close shuts down all underlying tier limiters
func (tl *TierLimiter) Close() error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return nil
	}
	tl.closed = true

	var firstErr error
	for _, lim := range tl.limiters {
		if err := lim.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Quota returns the quota configuration for a given tier
func (tl *TierLimiter) Quota(tier Tier) (TierQuota, bool) {
	tl.mu.RLock()
	defer tl.mu.RUnlock()
	q, ok := tl.quotas[tier]
	return q, ok
}

// ExtractClientIP retrieves the real client IP considering proxy headers
func ExtractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if ip != "" {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// DefaultTierResolver extracts client key and customer tier from HTTP request headers
func DefaultTierResolver(r *http.Request) (key string, tier Tier) {
	// 1. Check custom tier header (e.g. from authenticated gateway context or internal proxy)
	if customTier := r.Header.Get("X-Adroit-Tier"); customTier != "" {
		normalized := Tier(strings.ToLower(strings.TrimSpace(customTier)))
		switch normalized {
		case TierFree, TierPro, TierEnterprise, TierInternal:
			tier = normalized
		default:
			tier = TierFree
		}
	} else {
		tier = TierFree
	}

	// 2. Identify client key: API key takes priority, then Authorization bearer hash, then client IP
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		key = "apikey:" + strings.TrimSpace(apiKey)
		return key, tier
	}

	if auth := r.Header.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if len(token) > 16 {
			key = "token:" + token[:16]
			return key, tier
		}
	}

	// Fallback to client IP
	key = "ip:" + ExtractClientIP(r)
	return key, tier
}
