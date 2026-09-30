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

func TestLogging(t *testing.T) {
	t.Run("records request and response sizes and status", func(t *testing.T) {
		var logBuf bytes.Buffer
		log := logger.New(logger.Config{
			Format: "json",
			Output: &logBuf,
		})

		handler := middleware.Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created resource"))
		}))

		body := bytes.NewBufferString("input payload")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test-endpoint", body)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", rec.Code)
		}
		if rec.Body.String() != "created resource" {
			t.Fatalf("unexpected body: %s", rec.Body.String())
		}

		logOutput := logBuf.String()
		if !bytes.Contains(logBuf.Bytes(), []byte(`"bytes_in":13`)) {
			t.Errorf("expected bytes_in 13 in log output, got: %s", logOutput)
		}
		if !bytes.Contains(logBuf.Bytes(), []byte(`"bytes_out":16`)) {
			t.Errorf("expected bytes_out 16 in log output, got: %s", logOutput)
		}
		if !bytes.Contains(logBuf.Bytes(), []byte(`"status":201`)) {
			t.Errorf("expected status 201 in log output, got: %s", logOutput)
		}
		if !bytes.Contains(logBuf.Bytes(), []byte(`"method":"POST"`)) {
			t.Errorf("expected method POST in log output, got: %s", logOutput)
		}
	})

	t.Run("handles implicit status 200 on write", func(t *testing.T) {
		var logBuf bytes.Buffer
		log := logger.New(logger.Config{
			Format: "json",
			Output: &logBuf,
		})

		handler := middleware.Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		logOutput := logBuf.String()
		if !bytes.Contains(logBuf.Bytes(), []byte(`"bytes_out":2`)) {
			t.Errorf("expected bytes_out 2 in log output, got: %s", logOutput)
		}
		if !bytes.Contains(logBuf.Bytes(), []byte(`"bytes_in":0`)) {
			t.Errorf("expected bytes_in 0 in log output, got: %s", logOutput)
		}
		if !bytes.Contains(logBuf.Bytes(), []byte(`"status":200`)) {
			t.Errorf("expected status 200 in log output, got: %s", logOutput)
		}
	})
}

