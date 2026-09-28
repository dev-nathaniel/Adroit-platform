package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestAPIGatewayRouter(t *testing.T) {
	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	rl := middleware.NewRateLimiter(5.0, 5)
	defer rl.Stop()

	router := SetupRouter(log, rl)

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
		strictRouter := SetupRouter(log, strictRL)

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
}
