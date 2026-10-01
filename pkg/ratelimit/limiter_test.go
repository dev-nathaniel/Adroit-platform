package ratelimit_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestMemoryLimiterTokenBucket(t *testing.T) {
	ctx := context.Background()
	lim := ratelimit.NewMemoryLimiter(ratelimit.Config{
		Algorithm: ratelimit.AlgorithmTokenBucket,
		Rate:      10.0,
		Burst:     3,
	})
	defer lim.Close()

	// 1. Consume all 3 burst tokens
	for i := 1; i <= 3; i++ {
		res, err := lim.Allow(ctx, "client-1")
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
		expectedRemaining := int64(3 - i)
		if res.Remaining != expectedRemaining {
			t.Errorf("request %d: expected remaining %d, got %d", i, expectedRemaining, res.Remaining)
		}
	}

	// 2. 4th request must be rejected
	res4, err := lim.Allow(ctx, "client-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.Allowed {
		t.Fatalf("expected 4th request to be denied")
	}
	if res4.Remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", res4.Remaining)
	}
	if res4.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter, got %v", res4.RetryAfter)
	}

	// 3. Independent key should be allowed
	resOther, err := lim.Allow(ctx, "client-2")
	if err != nil || !resOther.Allowed {
		t.Fatalf("independent key client-2 should be allowed")
	}

	// 4. Token refill after wait
	time.Sleep(120 * time.Millisecond) // 10 tokens/sec = 1 token per 100ms
	resRefilled, err := lim.Allow(ctx, "client-1")
	if err != nil {
		t.Fatalf("unexpected error after refill: %v", err)
	}
	if !resRefilled.Allowed {
		t.Fatalf("expected request to be allowed after token refill")
	}

	// 5. Test Reset
	_ = lim.Reset(ctx, "client-1")
	resReset, err := lim.Allow(ctx, "client-1")
	if err != nil || !resReset.Allowed {
		t.Fatalf("expected key to be fully allowed after Reset")
	}
	if resReset.Remaining != 2 {
		t.Errorf("expected remaining 2 after reset and 1 consumed, got %d", resReset.Remaining)
	}
}

func TestMemoryLimiterSlidingWindow(t *testing.T) {
	ctx := context.Background()
	lim := ratelimit.NewMemoryLimiter(ratelimit.Config{
		Algorithm: ratelimit.AlgorithmSlidingWindow,
		Burst:     2,
		Window:    200 * time.Millisecond,
	})
	defer lim.Close()

	// 1. Consume 2 requests
	for i := 1; i <= 2; i++ {
		res, err := lim.Allow(ctx, "sw-user")
		if err != nil || !res.Allowed {
			t.Fatalf("request %d expected allowed", i)
		}
	}

	// 2. 3rd request denied
	resDenied, err := lim.Allow(ctx, "sw-user")
	if err != nil || resDenied.Allowed {
		t.Fatalf("3rd request should be denied")
	}

	// 3. Wait for sliding window to expire
	time.Sleep(250 * time.Millisecond)

	resAfter, err := lim.Allow(ctx, "sw-user")
	if err != nil || !resAfter.Allowed {
		t.Fatalf("request should be allowed after window passes")
	}
}

func TestRedisLimiterTokenBucket(t *testing.T) {
	// Connect to local Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("local redis not reachable, skipping live redis test")
	}
	_ = rdb.Close()

	lim, err := ratelimit.NewRedisLimiter(ratelimit.Config{
		Backend:          ratelimit.BackendRedis,
		Algorithm:        ratelimit.AlgorithmTokenBucket,
		Rate:             10.0,
		Burst:            3,
		KeyPrefix:        "test:rl:",
		RedisAddrs:       []string{"localhost:6379"},
		FallbackToMemory: true,
	})
	if err != nil {
		t.Fatalf("failed to create RedisLimiter: %v", err)
	}
	defer lim.Close()

	testKey := "redis-user-" + time.Now().Format("150405.000")
	_ = lim.Reset(ctx, testKey)

	// Consume burst of 3
	for i := 1; i <= 3; i++ {
		res, err := lim.Allow(ctx, testKey)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("request %d expected allowed", i)
		}
	}

	// 4th request should be denied
	res4, err := lim.Allow(ctx, testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.Allowed {
		t.Fatalf("expected 4th request to be denied on redis")
	}
	if res4.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter from redis script, got %v", res4.RetryAfter)
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)
	resRefilled, err := lim.Allow(ctx, testKey)
	if err != nil || !resRefilled.Allowed {
		t.Fatalf("expected request allowed after token refill on redis")
	}
}

func TestRedisLimiterSlidingWindow(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("local redis not reachable, skipping live redis test")
	}
	_ = rdb.Close()

	lim, err := ratelimit.NewRedisLimiter(ratelimit.Config{
		Backend:          ratelimit.BackendRedis,
		Algorithm:        ratelimit.AlgorithmSlidingWindow,
		Burst:            2,
		Window:           300 * time.Millisecond,
		KeyPrefix:        "test:sw:",
		RedisAddrs:       []string{"localhost:6379"},
		FallbackToMemory: true,
	})
	if err != nil {
		t.Fatalf("failed to create RedisLimiter: %v", err)
	}
	defer lim.Close()

	testKey := "redis-sw-" + time.Now().Format("150405.000")
	_ = lim.Reset(ctx, testKey)

	for i := 1; i <= 2; i++ {
		res, err := lim.Allow(ctx, testKey)
		if err != nil || !res.Allowed {
			t.Fatalf("request %d expected allowed on redis sliding window", i)
		}
	}

	// 3rd denied
	res3, err := lim.Allow(ctx, testKey)
	if err != nil || res3.Allowed {
		t.Fatalf("expected 3rd request denied on sliding window")
	}

	// Wait for window to roll
	time.Sleep(350 * time.Millisecond)
	resAfter, err := lim.Allow(ctx, testKey)
	if err != nil || !resAfter.Allowed {
		t.Fatalf("expected request allowed after sliding window reset")
	}
}

func TestRedisLimiterFallbackToMemory(t *testing.T) {
	ctx := context.Background()
	// Deliberately unreachable port
	lim, err := ratelimit.NewRedisLimiter(ratelimit.Config{
		Backend:          ratelimit.BackendRedis,
		Rate:             5.0,
		Burst:            2,
		RedisAddrs:       []string{"127.0.0.1:59999"},
		FallbackToMemory: true,
	})
	if err != nil {
		t.Fatalf("limiter creation with FallbackToMemory should not error: %v", err)
	}
	defer lim.Close()

	// Should still work seamlessly via memory fallback
	res1, err := lim.Allow(ctx, "fb-client")
	if err != nil || !res1.Allowed {
		t.Fatalf("request 1 should succeed via fallback: %v", err)
	}

	res2, err := lim.Allow(ctx, "fb-client")
	if err != nil || !res2.Allowed {
		t.Fatalf("request 2 should succeed via fallback: %v", err)
	}

	res3, err := lim.Allow(ctx, "fb-client")
	if err != nil || res3.Allowed {
		t.Fatalf("request 3 should be blocked even under fallback")
	}
}

func TestTierLimiter(t *testing.T) {
	ctx := context.Background()
	tl, err := ratelimit.NewTierLimiter(ratelimit.Config{
		Backend: ratelimit.BackendMemory,
	}, nil)
	if err != nil {
		t.Fatalf("failed to create TierLimiter: %v", err)
	}
	defer tl.Close()

	// Free tier quota check
	freeQuota, ok := tl.Quota(ratelimit.TierFree)
	if !ok || freeQuota.Burst != 30 {
		t.Errorf("unexpected free tier quota: %+v", freeQuota)
	}

	// Pro tier quota check
	proQuota, ok := tl.Quota(ratelimit.TierPro)
	if !ok || proQuota.Burst != 250 {
		t.Errorf("unexpected pro tier quota: %+v", proQuota)
	}

	// Allow on Free tier
	resFree, err := tl.Allow(ctx, ratelimit.TierFree, "tenant-1")
	if err != nil || !resFree.Allowed {
		t.Fatalf("expected Free tier request allowed")
	}
	if resFree.Limit != 30 {
		t.Errorf("expected Free tier limit 30, got %d", resFree.Limit)
	}

	// Allow on Pro tier
	resPro, err := tl.Allow(ctx, ratelimit.TierPro, "tenant-2")
	if err != nil || !resPro.Allowed {
		t.Fatalf("expected Pro tier request allowed")
	}
	if resPro.Limit != 250 {
		t.Errorf("expected Pro tier limit 250, got %d", resPro.Limit)
	}
}

func TestDefaultTierResolver(t *testing.T) {
	// 1. Explicit Tier Header
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("X-Adroit-Tier", "pro")
	req.Header.Set("X-API-Key", "sk_live_1234567890")
	key, tier := ratelimit.DefaultTierResolver(req)
	if tier != ratelimit.TierPro {
		t.Errorf("expected tier pro, got %s", tier)
	}
	if key != "apikey:sk_live_1234567890" {
		t.Errorf("expected apikey:sk_live_1234567890, got %s", key)
	}

	// 2. Bearer token
	reqBearer := httptest.NewRequest("GET", "/api/v1/test", nil)
	reqBearer.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz")
	reqBearer.RemoteAddr = "1.2.3.4:1234"
	keyBearer, tierBearer := ratelimit.DefaultTierResolver(reqBearer)
	if tierBearer != ratelimit.TierFree {
		t.Errorf("expected default tier free, got %s", tierBearer)
	}
	if keyBearer != "token:eyJhbGciOiJIUzI1" {
		t.Errorf("expected token prefix key, got %s", keyBearer)
	}

	// 3. Fallback to IP
	reqIP := httptest.NewRequest("GET", "/api/v1/test", nil)
	reqIP.RemoteAddr = "192.168.1.100:45678"
	keyIP, tierIP := ratelimit.DefaultTierResolver(reqIP)
	if tierIP != ratelimit.TierFree {
		t.Errorf("expected tier free, got %s", tierIP)
	}
	if keyIP != "ip:192.168.1.100" {
		t.Errorf("expected ip:192.168.1.100, got %s", keyIP)
	}
}

func TestConcurrentStressRateLimiting(t *testing.T) {
	ctx := context.Background()
	lim := ratelimit.NewMemoryLimiter(ratelimit.Config{
		Rate:  1000.0,
		Burst: 100,
	})
	defer lim.Close()

	var wg sync.WaitGroup
	var allowedCount int64
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				res, err := lim.Allow(ctx, "stress-key")
				if err == nil && res.Allowed {
					mu.Lock()
					allowedCount++
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	if allowedCount > 100 {
		t.Errorf("concurrent requests exceeded burst capacity: allowed %d, burst 100", allowedCount)
	}
}
