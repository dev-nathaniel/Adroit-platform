package logservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

const testJWTSecret = "log-service-test-jwt-secret-key-32b!"

func setupTestServer(t *testing.T) (*httptest.Server, *Service, *MockRepository, string) {
	repo := NewMockRepository()
	broadcaster := NewBroadcaster()
	eventBus := events.NewInMemBus()
	tailer := NewSimulatedLogTailer(100*time.Millisecond, false)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewService(repo, broadcaster, tailer, eventBus, logger)

	// Pre-seed a service
	svcRecord := &types.Service{
		ID:             "svc-handler-123",
		OrganizationID: "org-test-1",
		Name:           "web-frontend",
		Slug:           "web-frontend",
	}
	repo.AddService(svcRecord)
	repo.SetUserAccess("user-alice", "org-test-1", true)

	token, err := identity.GenerateJWT("user-alice", "alice@example.com", testJWTSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r, testJWTSecret)

	ts := httptest.NewServer(r)
	return ts, svc, repo, token
}

func TestHandler_Healthz(t *testing.T) {
	ts, _, _, _ := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandler_IngestAndGetLogs(t *testing.T) {
	ts, _, _, token := setupTestServer(t)
	defer ts.Close()

	// 1. Ingest batch logs
	ingestPayload := IngestLogsRequest{
		DeploymentID: "dep-100",
		Logs: []IngestLogItem{
			{Message: "Starting server on port 3000", Stream: "stdout"},
			{Message: "Failed to connect to redis cache", Stream: "stderr"},
		},
	}
	body, _ := json.Marshal(ingestPayload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/services/svc-handler-123/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}

	// 2. Query historical logs via REST
	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/services/svc-handler-123/logs?limit=10", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)

	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatalf("get logs failed: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getResp.StatusCode)
	}

	var apiRes response.APIResponse[types.AppLogsResponse]
	if err := json.NewDecoder(getResp.Body).Decode(&apiRes); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !apiRes.Success || len(apiRes.Data.Logs) != 2 {
		t.Errorf("expected 2 logs in response, got %+v", apiRes.Data)
	}
}

func TestHandler_BatchIngestEndpoint(t *testing.T) {
	ts, _, _, _ := setupTestServer(t)
	defer ts.Close()

	ingestPayload := IngestLogsRequest{
		ServiceID: "svc-handler-123",
		Logs: []IngestLogItem{
			{Message: "Global ingest test line"},
		},
	}
	body, _ := json.Marshal(ingestPayload)

	resp, err := http.Post(ts.URL+"/api/v1/logs/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("global ingest failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d", resp.StatusCode)
	}
}

func TestHandler_StreamLogsSSE(t *testing.T) {
	ts, svc, _, token := setupTestServer(t)
	defer ts.Close()

	// Pre-seed a log for tailing
	_ = svc.IngestLogs(context.Background(), "svc-handler-123", IngestLogsRequest{
		Logs: []IngestLogItem{{Message: "historical message"}},
	})

	// Open SSE stream with tail=10 and query parameter ?token=
	sseURL := fmt.Sprintf("%s/api/v1/services/svc-handler-123/logs/stream?token=%s&tail=10", ts.URL, token)
	req, _ := http.NewRequest(http.MethodGet, sseURL, nil)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Read first line (pre-seeded historical log)
	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read SSE failed: %v", err)
	}
	output := string(buf[:n])
	if !strings.Contains(output, "historical message") {
		t.Errorf("expected historical message in SSE stream, got: %s", output)
	}
}

func TestHandler_StreamLogsWebSocket(t *testing.T) {
	ts, svc, _, token := setupTestServer(t)
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/services/svc-handler-123/logs/ws?token=" + token

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v, resp: %+v", err, resp)
	}
	defer conn.Close()

	// 1. First message should be type "connected"
	var welcomeMsg WebSocketServerMessage
	if err := conn.ReadJSON(&welcomeMsg); err != nil {
		t.Fatalf("failed to read welcome message: %v", err)
	}
	if welcomeMsg.Type != "connected" {
		t.Errorf("expected 'connected' message, got %s", welcomeMsg.Type)
	}

	// 2. Ingest live log asynchronously
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = svc.IngestLogs(context.Background(), "svc-handler-123", IngestLogsRequest{
			Logs: []IngestLogItem{{Message: "live websocket message"}},
		})
	}()

	// 3. Read live log frame from WebSocket
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var logMsg WebSocketServerMessage
	if err := conn.ReadJSON(&logMsg); err != nil {
		t.Fatalf("failed to read live log from WS: %v", err)
	}
	if logMsg.Type != "log" || logMsg.Log == nil || logMsg.Log.Message != "live websocket message" {
		t.Errorf("unexpected log message received over WS: %+v", logMsg)
	}

	// 4. Send ping and verify pong
	if err := conn.WriteJSON(WebSocketClientMessage{Action: "ping"}); err != nil {
		t.Fatalf("failed to write ping: %v", err)
	}

	var pongMsg WebSocketServerMessage
	if err := conn.ReadJSON(&pongMsg); err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}
	if pongMsg.Type != "pong" {
		t.Errorf("expected pong, got: %s", pongMsg.Type)
	}
}

func TestHandler_PurgeLogs(t *testing.T) {
	ts, svc, _, token := setupTestServer(t)
	defer ts.Close()

	_ = svc.IngestLogs(context.Background(), "svc-handler-123", IngestLogsRequest{
		Logs: []IngestLogItem{{Message: "to be purged"}},
	})

	futureTime := url.QueryEscape(time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339))
	reqURL := fmt.Sprintf("%s/api/v1/services/svc-handler-123/logs?before=%s", ts.URL, futureTime)

	req, _ := http.NewRequest(http.MethodDelete, reqURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("purge request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}
