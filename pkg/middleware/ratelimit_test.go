package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestRateLimiting(t *testing.T) {
	// Rate limiter with 2 tokens per second and burst of 3
	rl := middleware.NewRateLimiter(2.0, 3)
	defer rl.Stop()

	handler := middleware.RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Consume all 3 burst tokens
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "192.168.1.50:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d", i, rec.Code)
		}

		if rec.Header().Get("X-RateLimit-Limit") != "3" {
			t.Errorf("expected X-RateLimit-Limit 3")
		}
	}

	// 2. 4th request must be rate limited (429)
	reqExceeded := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqExceeded.RemoteAddr = "192.168.1.50:12345"
	recExceeded := httptest.NewRecorder()

	handler.ServeHTTP(recExceeded, reqExceeded)

	if recExceeded.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 Too Many Requests, got %d", recExceeded.Code)
	}

	if recExceeded.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header to be present")
	}

	var res response.APIResponse[any]
	if err := json.Unmarshal(recExceeded.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode 429 response body: %v", err)
	}

	if res.Success || res.Error == nil || res.Error.Code != response.ErrCodeRateLimited {
		t.Errorf("expected RATE_LIMITED error code, got %+v", res.Error)
	}

	// 3. Different IP must not be blocked (independent bucket)
	reqDifferentIP := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqDifferentIP.RemoteAddr = "10.0.0.1:54321"
	recDifferentIP := httptest.NewRecorder()

	handler.ServeHTTP(recDifferentIP, reqDifferentIP)

	if recDifferentIP.Code != http.StatusOK {
		t.Fatalf("expected status 200 for different IP, got %d", recDifferentIP.Code)
	}

	// 4. Test proxy header X-Forwarded-For
	reqProxy := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	reqProxy.Header.Set("X-Forwarded-For", "203.0.113.195, 192.168.1.1")
	reqProxy.RemoteAddr = "127.0.0.1:8080"
	recProxy := httptest.NewRecorder()

	handler.ServeHTTP(recProxy, reqProxy)

	if recProxy.Code != http.StatusOK {
		t.Fatalf("expected status 200 for forwarded client IP, got %d", recProxy.Code)
	}
}

func TestRateLimiterRefill(t *testing.T) {
	// High fill rate for fast test: 10 tokens/sec, burst 1
	rl := middleware.NewRateLimiter(10.0, 1)
	defer rl.Stop()

	allowed, rem, _ := rl.Allow("1.2.3.4")
	if !allowed || rem != 0 {
		t.Fatalf("expected first request allowed with 0 remaining")
	}

	// Immediate next request should fail
	allowed2, _, _ := rl.Allow("1.2.3.4")
	if allowed2 {
		t.Fatalf("expected immediate second request to be blocked")
	}

	// Wait 150ms for at least 1 token (10/sec = 1 per 100ms)
	time.Sleep(150 * time.Millisecond)

	allowed3, _, _ := rl.Allow("1.2.3.4")
	if !allowed3 {
		t.Fatalf("expected request allowed after token refill")
	}
}
