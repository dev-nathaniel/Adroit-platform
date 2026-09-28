package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestAPIGatewayRouter(t *testing.T) {
	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	router := SetupRouter(log)

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
}
