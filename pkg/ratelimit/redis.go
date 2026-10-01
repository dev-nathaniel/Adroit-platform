package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Redis Lua script for token bucket rate limiting.
// Uses hash tags {key} to guarantee Redis Cluster compatibility.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local cost = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local rate = tonumber(ARGV[3])
local now = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

local data = redis.call("HMGET", key, "tokens", "last_refill")
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if not tokens or not last_refill then
    tokens = burst
    last_refill = now
else
    local elapsed = math.max(0, now - last_refill)
    tokens = math.min(burst, tokens + (elapsed * rate))
    last_refill = now
end

local allowed = 0
local remaining = 0
local retry_after = 0

if tokens >= cost then
    tokens = tokens - cost
    allowed = 1
    remaining = math.floor(tokens)
else
    allowed = 0
    remaining = 0
    local missing = cost - tokens
    retry_after = missing / rate
end

redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
redis.call("EXPIRE", key, ttl)

return {allowed, remaining, tostring(tokens), tostring(retry_after)}
`)

// Redis Lua script for sliding window rate limiting.
// Uses sorted sets (ZSET) with millisecond timestamps and hash tags.
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])
local member_prefix = ARGV[5]

local clear_before = now - window
redis.call("ZREMRANGEBYSCORE", key, "-inf", clear_before)

local current_count = redis.call("ZCARD", key)
local allowed = 0
local remaining = 0
local retry_after = 0

if current_count + cost <= limit then
    for i = 1, cost do
        redis.call("ZADD", key, now, member_prefix .. ":" .. i)
    end
    allowed = 1
    remaining = limit - (current_count + cost)
else
    allowed = 0
    remaining = 0
    local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
    if #oldest >= 2 then
        local oldest_score = tonumber(oldest[2])
        local retry_ms = (oldest_score + window) - now
        if retry_ms > 0 then
            retry_after = retry_ms / 1000
        end
    else
        retry_after = window / 1000
    end
end

local ttl = math.max(60, math.ceil((window / 1000) * 2))
redis.call("EXPIRE", key, ttl)

return {allowed, remaining, tostring(limit), tostring(retry_after)}
`)

// RedisLimiter implements Limiter backed by Redis / Redis Cluster
type RedisLimiter struct {
	client        redis.UniversalClient
	ownsClient    bool
	cfg           Config
	memoryFallback *MemoryLimiter
	logger        *slog.Logger
	mu            sync.RWMutex
	closed        bool

	// Health status tracking
	isDown        atomic.Bool
	lastWarnTime  time.Time
	warnMu        sync.Mutex
}

// NewRedisLimiter creates a distributed rate limiter backed by Redis or Redis Cluster
func NewRedisLimiter(cfg Config) (*RedisLimiter, error) {
	if cfg.Rate <= 0 {
		cfg.Rate = 100.0
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 200
	}
	if cfg.Window <= 0 {
		cfg.Window = 1 * time.Minute
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "adroit:rl:"
	}
	if len(cfg.RedisAddrs) == 0 {
		cfg.RedisAddrs = []string{"localhost:6379"}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var client redis.UniversalClient
	var ownsClient bool

	if cfg.RedisClient != nil {
		client = cfg.RedisClient
		ownsClient = false
	} else {
		ownsClient = true
		client = redis.NewUniversalClient(&redis.UniversalOptions{
			Addrs:    cfg.RedisAddrs,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
	}

	rl := &RedisLimiter{
		client:        client,
		ownsClient:    ownsClient,
		cfg:           cfg,
		memoryFallback: NewMemoryLimiter(cfg),
		logger:        logger,
	}

	// Ping test with 2s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		rl.isDown.Store(true)
		if !cfg.FallbackToMemory {
			return nil, fmt.Errorf("%w: failed to ping redis: %v", ErrRedisUnavailable, err)
		}
		logger.Warn("Redis cluster/instance is currently unreachable, engaged in-memory fallback",
			slog.String("addrs", strings.Join(cfg.RedisAddrs, ",")),
			slog.String("error", err.Error()),
		)
	} else {
		rl.isDown.Store(false)
		logger.Info("Distributed rate limiter connected to Redis backend",
			slog.String("addrs", strings.Join(cfg.RedisAddrs, ",")),
			slog.String("algorithm", string(cfg.Algorithm)),
			slog.Float64("rate", cfg.Rate),
			slog.Int64("burst", cfg.Burst),
		)
	}

	return rl, nil
}

// formatKey formats the Redis key with hash tags {key} for Redis Cluster shard routing
func (rl *RedisLimiter) formatKey(key string) string {
	// e.g. adroit:rl:{user_123}
	return fmt.Sprintf("%s{%s}", rl.cfg.KeyPrefix, key)
}

// Allow checks if 1 request is allowed for the key
func (rl *RedisLimiter) Allow(ctx context.Context, key string) (*Result, error) {
	return rl.AllowN(ctx, key, 1)
}

// AllowN checks if n requests are allowed for the key
func (rl *RedisLimiter) AllowN(ctx context.Context, key string, n int64) (*Result, error) {
	if key == "" {
		return nil, ErrKeyEmpty
	}

	rl.mu.RLock()
	if rl.closed {
		rl.mu.RUnlock()
		return nil, ErrLimiterClosed
	}
	rl.mu.RUnlock()

	// If marked down and fallback enabled, bypass Redis directly
	if rl.isDown.Load() && rl.cfg.FallbackToMemory {
		return rl.memoryFallback.AllowN(ctx, key, n)
	}

	redisKey := rl.formatKey(key)

	var res *Result
	var err error

	if rl.cfg.Algorithm == AlgorithmSlidingWindow {
		res, err = rl.evalSlidingWindow(ctx, redisKey, n)
	} else {
		res, err = rl.evalTokenBucket(ctx, redisKey, n)
	}

	if err != nil {
		// Redis error encountered - log and fallback
		rl.handleRedisFailure(err)
		if rl.cfg.FallbackToMemory {
			return rl.memoryFallback.AllowN(ctx, key, n)
		}
		return nil, err
	}

	// Successfully executed on Redis
	if rl.isDown.Swap(false) {
		rl.logger.Info("Redis rate limiter reconnected and resumed distributed synchronization")
	}

	return res, nil
}

func (rl *RedisLimiter) evalTokenBucket(ctx context.Context, redisKey string, cost int64) (*Result, error) {
	now := float64(time.Now().UnixNano()) / 1e9 // seconds with float precision
	ttl := 3600                                 // 1 hour bucket idle expiration

	raw, err := tokenBucketScript.Run(
		ctx,
		rl.client,
		[]string{redisKey},
		cost,
		rl.cfg.Burst,
		rl.cfg.Rate,
		now,
		ttl,
	).Result()

	if err != nil {
		return nil, err
	}

	slice, ok := raw.([]any)
	if !ok || len(slice) < 4 {
		return nil, fmt.Errorf("unexpected redis script response format: %v", raw)
	}

	allowedInt, _ := slice[0].(int64)
	remainingInt, _ := slice[1].(int64)
	tokensStr, _ := slice[2].(string)
	retryAfterStr, _ := slice[3].(string)

	tokens, _ := strconv.ParseFloat(tokensStr, 64)
	retryAfterSec, _ := strconv.ParseFloat(retryAfterStr, 64)

	allowed := allowedInt == 1
	var retryAfter time.Duration
	if !allowed {
		retryAfter = time.Duration(retryAfterSec*float64(time.Second)) + (50 * time.Millisecond)
	}

	missing := float64(rl.cfg.Burst) - tokens
	resetAfter := time.Duration((missing / rl.cfg.Rate) * float64(time.Second))

	return &Result{
		Allowed:    allowed,
		Limit:      rl.cfg.Burst,
		Remaining:  remainingInt,
		ResetAfter: resetAfter,
		RetryAfter: retryAfter,
	}, nil
}

func (rl *RedisLimiter) evalSlidingWindow(ctx context.Context, redisKey string, cost int64) (*Result, error) {
	nowMs := time.Now().UnixNano() / 1e6
	windowMs := rl.cfg.Window.Milliseconds()
	memberPrefix := fmt.Sprintf("%d:%s", nowMs, uuid.NewString()[:8])

	raw, err := slidingWindowScript.Run(
		ctx,
		rl.client,
		[]string{redisKey},
		nowMs,
		windowMs,
		rl.cfg.Burst,
		cost,
		memberPrefix,
	).Result()

	if err != nil {
		return nil, err
	}

	slice, ok := raw.([]any)
	if !ok || len(slice) < 4 {
		return nil, fmt.Errorf("unexpected redis script response format: %v", raw)
	}

	allowedInt, _ := slice[0].(int64)
	remainingInt, _ := slice[1].(int64)
	retryAfterStr, _ := slice[3].(string)

	retryAfterSec, _ := strconv.ParseFloat(retryAfterStr, 64)

	allowed := allowedInt == 1
	var retryAfter time.Duration
	if !allowed {
		retryAfter = time.Duration(retryAfterSec*float64(time.Second)) + (50 * time.Millisecond)
	}

	return &Result{
		Allowed:    allowed,
		Limit:      rl.cfg.Burst,
		Remaining:  remainingInt,
		ResetAfter: rl.cfg.Window,
		RetryAfter: retryAfter,
	}, nil
}

func (rl *RedisLimiter) handleRedisFailure(err error) {
	rl.isDown.Store(true)

	rl.warnMu.Lock()
	defer rl.warnMu.Unlock()

	// Rate-limit warning logs to once every 10 seconds to avoid log spam
	if time.Since(rl.lastWarnTime) > 10*time.Second {
		rl.lastWarnTime = time.Now()
		rl.logger.Warn("Redis rate limiting error, falling back to local memory limiter",
			slog.String("error", err.Error()),
		)
	}
}

// Reset clears rate limit state for a key
func (rl *RedisLimiter) Reset(ctx context.Context, key string) error {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return ErrLimiterClosed
	}

	_ = rl.memoryFallback.Reset(ctx, key)

	if !rl.isDown.Load() {
		redisKey := rl.formatKey(key)
		return rl.client.Del(ctx, redisKey).Err()
	}
	return nil
}

// Close closes the Redis client if owned and terminates memory fallback
func (rl *RedisLimiter) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.closed {
		return nil
	}
	rl.closed = true

	_ = rl.memoryFallback.Close()

	if rl.ownsClient && rl.client != nil {
		return rl.client.Close()
	}
	return nil
}
