package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/logger"
)

func TestLoggerWithRequestID(t *testing.T) {
	buf := new(bytes.Buffer)
	log := logger.New(logger.Config{
		Level:  "info",
		Format: "json",
		Output: buf,
	})

	ctx := logger.WithRequestID(context.Background(), "req-test-12345")
	log.InfoContext(ctx, "hello test")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse log json output: %v, raw: %s", err, buf.String())
	}

	if parsed["msg"] != "hello test" {
		t.Errorf("expected msg 'hello test', got '%v'", parsed["msg"])
	}

	if parsed["request_id"] != "req-test-12345" {
		t.Errorf("expected request_id 'req-test-12345', got '%v'", parsed["request_id"])
	}
}

func TestGetRequestID(t *testing.T) {
	ctx := context.Background()
	if id := logger.GetRequestID(ctx); id != "" {
		t.Errorf("expected empty id, got '%s'", id)
	}

	ctx = logger.WithRequestID(ctx, "req-xyz")
	if id := logger.GetRequestID(ctx); id != "req-xyz" {
		t.Errorf("expected 'req-xyz', got '%s'", id)
	}
}
