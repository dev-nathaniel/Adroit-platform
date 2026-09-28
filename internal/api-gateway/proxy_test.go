package apigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	apigateway "github.com/dev-nathaniel/Adroit-platform/internal/api-gateway"
	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestReverseProxySuccess(t *testing.T) {
	// Mock downstream service
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		response.JSON(w, http.StatusOK, map[string]string{
			"message":    "from downstream",
			"request_id": reqID,
		})
	}))
	defer downstream.Close()

	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	targetURL, _ := url.Parse(downstream.URL)

	proxy, err := apigateway.NewReverseProxy(apigateway.ReverseProxyConfig{
		ServiceName: "test-service",
		TargetURL:   targetURL,
		Log:         log,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx := logger.WithRequestID(req.Context(), "req_proxy_123")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var res response.APIResponse[map[string]string]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Data["request_id"] != "req_proxy_123" {
		t.Errorf("expected propagated request_id 'req_proxy_123', got '%s'", res.Data["request_id"])
	}
}

func TestReverseProxyDownstreamUnavailable(t *testing.T) {
	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	// Non-listening local port to trigger connection refused
	deadURL, _ := url.Parse("http://127.0.0.1:54321")

	proxy, err := apigateway.NewReverseProxy(apigateway.ReverseProxyConfig{
		ServiceName: "dead-service",
		TargetURL:   deadURL,
		Log:         log,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dead", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d", rec.Code)
	}

	var res response.APIResponse[any]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode 502 response: %v", err)
	}

	if res.Success || res.Error == nil || res.Error.Code != response.ErrCodeServiceUnavailable {
		t.Errorf("expected SERVICE_UNAVAILABLE error code, got %+v", res.Error)
	}
}
