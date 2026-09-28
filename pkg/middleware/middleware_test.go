package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
	"github.com/dev-nathaniel/Adroit-platform/pkg/middleware"
)

func TestRequestID(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := logger.GetRequestID(r.Context())
		if reqID == "" {
			t.Errorf("expected request_id in context, got empty")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	handler.ServeHTTP(rec, req)

	headerVal := rec.Header().Get("X-Request-ID")
	if headerVal == "" {
		t.Errorf("expected X-Request-ID header in response")
	}
}

func TestRecovery(t *testing.T) {
	log := logger.New(logger.Config{Output: new(bytes.Buffer)})
	panicHandler := middleware.Recovery(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated fatal crash")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)

	// Must not crash the process
	panicHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 status code on panic recovery, got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	corsHandler := middleware.CORS("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// OPTIONS preflight
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	corsHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for preflight, got %d", rec.Code)
	}

	if allowOrigin := rec.Header().Get("Access-Control-Allow-Origin"); allowOrigin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got '%s'", allowOrigin)
	}
}
