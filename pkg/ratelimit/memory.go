package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"
)

type memoryBucket struct {
	tokens     float64
	lastRefill time.Time
	// For sliding window
	timestamps []time.Time
}

// MemoryLimiter is a thread-safe, zero-dependency in-memory rate limiter
type MemoryLimiter struct {
	mu           sync.Mutex
	cfg          Config
	buckets      map[string]*memoryBucket
	cleanupTimer *time.Ticker
	stopCleanup  chan struct{}
	closed       bool
}

// NewMemoryLimiter creates a new MemoryLimiter
func NewMemoryLimiter(cfg Config) *MemoryLimiter {
	if cfg.Rate <= 0 {
		cfg.Rate = 100.0
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 200
	}
	if cfg.Window <= 0 {
		cfg.Window = 1 * time.Minute
	}
	if cfg.Algorithm == "" {
		cfg.Algorithm = AlgorithmTokenBucket
	}

	ml := &MemoryLimiter{
		cfg:          cfg,
		buckets:      make(map[string]*memoryBucket),
		cleanupTimer: time.NewTicker(2 * time.Minute),
		stopCleanup:  make(chan struct{}),
	}

	go ml.cleanupLoop()

	return ml
}

func (ml *MemoryLimiter) cleanupLoop() {
	for {
		select {
		case <-ml.cleanupTimer.C:
			ml.mu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for k, b := range ml.buckets {
				if b.lastRefill.Before(cutoff) {
					delete(ml.buckets, k)
				}
			}
			ml.mu.Unlock()
		case <-ml.stopCleanup:
			ml.cleanupTimer.Stop()
			return
		}
	}
}

// Allow checks if 1 request is permitted for the given key
func (ml *MemoryLimiter) Allow(ctx context.Context, key string) (*Result, error) {
	return ml.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are permitted for the given key
func (ml *MemoryLimiter) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	if key == "" {
		return nil, ErrKeyEmpty
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()

	if ml.closed {
		return nil, ErrLimiterClosed
	}

	now := time.Now()
	b, exists := ml.buckets[key]
	if !exists {
		b = &memoryBucket{
			tokens:     float64(ml.cfg.Burst),
			lastRefill: now,
		}
		ml.buckets[key] = b
	}

	if ml.cfg.Algorithm == AlgorithmSlidingWindow {
		return ml.allowSlidingWindow(b, now, n)
	}

	return ml.allowTokenBucket(b, now, n)
}

func (ml *MemoryLimiter) allowTokenBucket(b *memoryBucket, now time.Time, n int64) (*Result, error) {
	// Refill tokens based on elapsed duration
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * ml.cfg.Rate
		if b.tokens > float64(ml.cfg.Burst) {
			b.tokens = float64(ml.cfg.Burst)
		}
		b.lastRefill = now
	}

	cost := float64(n)
	burstLimit := ml.cfg.Burst

	if b.tokens >= cost {
		b.tokens -= cost
		remaining := int64(math.Floor(b.tokens))

		// ResetAfter is time until bucket reaches full capacity
		missingTokens := float64(burstLimit) - b.tokens
		resetAfter := time.Duration((missingTokens / ml.cfg.Rate) * float64(time.Second))

		return &Result{
			Allowed:    true,
			Limit:      burstLimit,
			Remaining:  remaining,
			ResetAfter: resetAfter,
			RetryAfter: 0,
		}, nil
	}

	// Insufficient tokens
	missingTokens := cost - b.tokens
	retrySeconds := missingTokens / ml.cfg.Rate
	retryAfter := time.Duration(retrySeconds*float64(time.Second)) + (50 * time.Millisecond)

	return &Result{
		Allowed:    false,
		Limit:      burstLimit,
		Remaining:  0,
		ResetAfter: retryAfter,
		RetryAfter: retryAfter,
	}, nil
}

func (ml *MemoryLimiter) allowSlidingWindow(b *memoryBucket, now time.Time, n int64) (*Result, error) {
	windowStart := now.Add(-ml.cfg.Window)
	valid := make([]time.Time, 0, len(b.timestamps))
	for _, t := range b.timestamps {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	b.timestamps = valid
	b.lastRefill = now

	limit := ml.cfg.Burst
	currentCount := int64(len(b.timestamps))

	if currentCount+n <= limit {
		for i := int64(0); i < n; i++ {
			b.timestamps = append(b.timestamps, now)
		}
		remaining := limit - (currentCount + n)
		return &Result{
			Allowed:    true,
			Limit:      limit,
			Remaining:  remaining,
			ResetAfter: ml.cfg.Window,
			RetryAfter: 0,
		}, nil
	}

	// Window full - calculate retry duration until oldest timestamp expires
	retryAfter := ml.cfg.Window
	if len(b.timestamps) > 0 {
		oldest := b.timestamps[0]
		expiresAt := oldest.Add(ml.cfg.Window)
		if expiresAt.After(now) {
			retryAfter = expiresAt.Sub(now)
		}
	}

	return &Result{
		Allowed:    false,
		Limit:      limit,
		Remaining:  0,
		ResetAfter: retryAfter,
		RetryAfter: retryAfter,
	}, nil
}

// Reset clears rate limit state for a key
func (ml *MemoryLimiter) Reset(ctx context.Context, key string) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	delete(ml.buckets, key)
	return nil
}

// Close gracefully stops the janitor goroutine
func (ml *MemoryLimiter) Close() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if !ml.closed {
		ml.closed = true
		close(ml.stopCleanup)
	}
	return nil
}
