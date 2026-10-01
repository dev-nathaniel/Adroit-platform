package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/ratelimit"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestAPIGatewayRouter(t *testing.T) {
	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	rl := middleware.NewRateLimiter(5.0, 5)
	defer rl.Stop()

	router := SetupRouter(log, rl, DownstreamConfig{})

	t.Run("GET /healthz returns 200 and healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var res response.APIResponse[map[string]any]
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !res.Success || res.Data["status"] != "healthy" {
			t.Errorf("expected success true and status healthy, got %+v", res)
		}
	})

	t.Run("GET / returns 200 with router chi/v5", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var res response.APIResponse[map[string]any]
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res.Data["router"] != "chi/v5" {
			t.Errorf("expected router 'chi/v5', got '%v'", res.Data["router"])
		}
	})

	t.Run("GET /api/v1/status returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("GET /unknown-route returns custom 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/unknown-route", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", rec.Code)
		}

		var res response.APIResponse[any]
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse 404 response: %v", err)
		}

		if res.Success || res.Error == nil || res.Error.Code != response.ErrCodeNotFound {
			t.Errorf("expected NOT_FOUND error code, got %+v", res.Error)
		}
	})

	t.Run("Exceeding rate limit returns 429 Too Many Requests", func(t *testing.T) {
		strictRL := middleware.NewRateLimiter(1.0, 1)
		defer strictRL.Stop()
		strictRouter := SetupRouter(log, strictRL, DownstreamConfig{})

		// 1st request should pass
		req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req1.RemoteAddr = "10.10.10.10:8000"
		rec1 := httptest.NewRecorder()
		strictRouter.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("expected first request 200, got %d", rec1.Code)
		}

		// 2nd immediate request from same IP should get 429
		req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req2.RemoteAddr = "10.10.10.10:8000"
		rec2 := httptest.NewRecorder()
		strictRouter.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusTooManyRequests {
			t.Fatalf("expected second request 429, got %d", rec2.Code)
		}

		if rec2.Header().Get("Retry-After") == "" {
			t.Errorf("expected Retry-After header on 429")
		}
	})

	t.Run("Proxy forwards traffic to downstream auth service", func(t *testing.T) {
		mockAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, http.StatusOK, map[string]string{
				"service": "auth-service",
				"path":    r.URL.Path,
			})
		}))
		defer mockAuth.Close()

		proxyRouter := SetupRouter(log, nil, DownstreamConfig{
			AuthServiceURL: mockAuth.URL,
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rec := httptest.NewRecorder()
		proxyRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on proxied request, got %d", rec.Code)
		}

		var res response.APIResponse[map[string]string]
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Data["service"] != "auth-service" {
			t.Errorf("expected service auth-service, got %s", res.Data["service"])
		}
	})

	t.Run("SetupRouter with distributed Limiter enforces limits and headers", func(t *testing.T) {
		memLim := ratelimit.NewMemoryLimiter(ratelimit.Config{
			Rate:  10.0,
			Burst: 2,
		})
		defer memLim.Close()

		distRouter := SetupRouter(log, memLim, DownstreamConfig{})

		for i := 1; i <= 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.RemoteAddr = "192.168.10.20:9000"
			rec := httptest.NewRecorder()
			distRouter.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("request %d expected status 200, got %d", i, rec.Code)
			}
			if rec.Header().Get("X-RateLimit-Limit") != "2" {
				t.Errorf("expected limit 2, got %s", rec.Header().Get("X-RateLimit-Limit"))
			}
		}

		// 3rd request should get 429
		req3 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req3.RemoteAddr = "192.168.10.20:9000"
		rec3 := httptest.NewRecorder()
		distRouter.ServeHTTP(rec3, req3)

		if rec3.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 Too Many Requests, got %d", rec3.Code)
		}
		if rec3.Header().Get("Retry-After") == "" {
			t.Errorf("expected Retry-After header")
		}
	})

	t.Run("SetupRouter with TierLimiter resolves customer tier and quota", func(t *testing.T) {
		tl, err := ratelimit.NewTierLimiter(ratelimit.Config{
			Backend: ratelimit.BackendMemory,
		}, nil)
		if err != nil {
			t.Fatalf("failed to create TierLimiter: %v", err)
		}
		defer tl.Close()

		tierRouter := SetupRouter(log, tl, DownstreamConfig{})

		// Pro tier request
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Adroit-Tier", "pro")
		req.Header.Set("X-API-Key", "live-pro-key-abc")
		rec := httptest.NewRecorder()

		tierRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Tier") != "pro" {
			t.Errorf("expected tier pro, got %s", rec.Header().Get("X-RateLimit-Tier"))
		}
		if rec.Header().Get("X-RateLimit-Limit") != "250" {
			t.Errorf("expected pro limit 250, got %s", rec.Header().Get("X-RateLimit-Limit"))
		}
	})
}

