package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_RequestAndErrorHandling(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Success endpoint
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid_jwt_token" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error": map[string]any{
					"code":    "UNAUTHORIZED",
					"message": "Invalid or missing token",
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":    "usr_123",
				"email": "test@adroit.run",
				"name":  "Test Developer",
			},
		})
	})

	// 2. Error endpoint
	mux.HandleFunc("/api/v1/projects/proj_999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "RESOURCE_NOT_FOUND",
				"message": "Project proj_999 not found",
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cli := New(server.URL, "valid_jwt_token")

	// Test successful Get
	user, err := cli.Me(context.Background())
	if err != nil {
		t.Fatalf("unexpected error fetching Me: %v", err)
	}
	if user.Email != "test@adroit.run" {
		t.Errorf("expected email test@adroit.run, got %s", user.Email)
	}

	// Test error Get
	_, err = cli.GetProject(context.Background(), "proj_999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "RESOURCE_NOT_FOUND" {
		t.Errorf("expected code RESOURCE_NOT_FOUND, got %s", apiErr.Code)
	}
}

func TestClient_StreamLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		w.Write([]byte(": keep-alive\n\n"))
		flusher.Flush()

		entry := LogEntry{
			ServiceID: "svc_123",
			Stream:    "stdout",
			Message:   "Application listening on :8080",
			Timestamp: time.Now(),
		}
		data, _ := json.Marshal(entry)
		w.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	cli := New(server.URL, "test_token")
	var received []LogEntry

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = cli.StreamLogs(ctx, "svc_123", LogStreamOptions{Tail: 10}, func(entry LogEntry) {
		received = append(received, entry)
		cancel() // Received message, terminate stream
	})

	if len(received) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(received))
	}
	if received[0].Message != "Application listening on :8080" {
		t.Errorf("unexpected message: %s", received[0].Message)
	}
}
