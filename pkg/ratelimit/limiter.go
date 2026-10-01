package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Algorithm defines the rate limiting algorithm used
type Algorithm string

const (
	AlgorithmTokenBucket   Algorithm = "token_bucket"
	AlgorithmSlidingWindow Algorithm = "sliding_window"
)

// BackendType defines the storage backend for rate limiting counters
type BackendType string

const (
	BackendMemory BackendType = "memory"
	BackendRedis  BackendType = "redis"
)

// Standard rate limit errors
var (
	ErrLimiterClosed     = errors.New("rate limiter is closed")
	ErrInvalidConfig     = errors.New("invalid rate limiter configuration")
	ErrKeyEmpty          = errors.New("rate limit key cannot be empty")
	ErrRedisUnavailable = errors.New("redis backend is unavailable")
)

// Result represents the outcome of an Allow check
type Result struct {
	Allowed    bool          `json:"allowed"`
	Limit      int64         `json:"limit"`
	Remaining  int64         `json:"remaining"`
	ResetAfter time.Duration `json:"reset_after"`
	RetryAfter time.Duration `json:"retry_after"`
}

// Limiter defines the contract for distributed and local rate limiters
type Limiter interface {
	// Allow checks if 1 request is allowed for the given key
	Allow(ctx context.Context, key string) (*Result, error)

	// AllowN checks if n requests are allowed for the given key
	AllowN(ctx context.Context, key string, n int64) (*Result, error)

	// Reset clears rate limit counters for a specific key
	Reset(ctx context.Context, key string) error

	// Close shuts down background routines or backend connections
	Close() error
}

// Config defines settings for initializing a Limiter
type Config struct {
	Backend          BackendType
	Algorithm        Algorithm
	Rate             float64       // Tokens per second (Token Bucket)
	Burst            int64         // Maximum burst capacity (Token Bucket) or Max Requests (Sliding Window)
	Window           time.Duration // Time window for Sliding Window or TTL for Token Bucket
	KeyPrefix        string        // Namespace prefix (default: "adroit:rl:")
	RedisAddrs       []string      // Host:Port address(es) for standalone or cluster Redis
	RedisPassword    string
	RedisDB          int
	RedisClient      redis.UniversalClient // Optional pre-configured universal client
	FallbackToMemory bool                  // If true and Redis fails, gracefully falls back to MemoryLimiter
	Logger           *slog.Logger
}

// DefaultConfig provides sensible platform defaults
func DefaultConfig() Config {
	return Config{
		Backend:          BackendMemory,
		Algorithm:        AlgorithmTokenBucket,
		Rate:             100.0,
		Burst:            200,
		Window:           1 * time.Minute,
		KeyPrefix:        "adroit:rl:",
		RedisAddrs:       []string{"localhost:6379"},
		FallbackToMemory: true,
	}
}

// New creates a new Limiter based on the supplied configuration
func New(cfg Config) (Limiter, error) {
	if cfg.Rate <= 0 && cfg.Burst <= 0 {
		return nil, fmt.Errorf("%w: rate and burst must be positive", ErrInvalidConfig)
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "adroit:rl:"
	}
	if cfg.Algorithm == "" {
		cfg.Algorithm = AlgorithmTokenBucket
	}

	switch cfg.Backend {
	case BackendRedis:
		return NewRedisLimiter(cfg)
	case BackendMemory, "":
		return NewMemoryLimiter(cfg), nil
	default:
		return nil, fmt.Errorf("%w: unknown backend type %q", ErrInvalidConfig, cfg.Backend)
	}
}
