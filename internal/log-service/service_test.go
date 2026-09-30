package logservice

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestService(t *testing.T) (*Service, *MockRepository, *Broadcaster, events.EventBus) {
	repo := NewMockRepository()
	broadcaster := NewBroadcaster()
	eventBus := events.NewInMemBus()
	tailer := NewSimulatedLogTailer(100*time.Millisecond, false)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewService(repo, broadcaster, tailer, eventBus, logger)

	// Pre-seed a service
	testSvc := &types.Service{
		ID:             "svc-test-123",
		OrganizationID: "org-1",
		Name:           "api-server",
		Slug:           "api-server",
	}
	repo.AddService(testSvc)
	repo.SetUserAccess("user-1", "org-1", true)
	repo.SetUserAccess("user-stranger", "org-1", false)

	return svc, repo, broadcaster, eventBus
}

func TestIngestLogs_Success(t *testing.T) {
	svc, repo, broadcaster, _ := setupTestService(t)
	ctx := context.Background()

	// Register a subscriber on the broadcaster to verify live fan-out
	subCh, unsub := broadcaster.Subscribe("svc-test-123")
	defer unsub()

	req := IngestLogsRequest{
		ServiceID:    "svc-test-123",
		DeploymentID: "dep-456",
		AllocationID: "alloc-789",
		Logs: []IngestLogItem{
			{Message: "Server boot complete", Stream: "stdout"},
			{Message: "Database migration error: syntax error", Stream: "stderr"},
			{Message: "Health check passing"},
		},
	}

	err := svc.IngestLogs(ctx, "svc-test-123", req)
	if err != nil {
		t.Fatalf("IngestLogs failed: %v", err)
	}

	// Verify entries in repo
	if len(repo.logs) != 3 {
		t.Fatalf("expected 3 logs in repo, got %d", len(repo.logs))
	}

	if repo.logs[0].Stream != "stdout" {
		t.Errorf("expected stream stdout, got %s", repo.logs[0].Stream)
	}
	if repo.logs[1].Stream != "stderr" {
		t.Errorf("expected stream stderr, got %s", repo.logs[1].Stream)
	}
	if repo.logs[2].Stream != "stdout" { // default
		t.Errorf("expected default stream stdout, got %s", repo.logs[2].Stream)
	}

	// Verify subscriber received the 3 broadcasts
	for i := 0; i < 3; i++ {
		select {
		case entry := <-subCh:
			if entry.ServiceID != "svc-test-123" {
				t.Errorf("expected serviceID svc-test-123, got %s", entry.ServiceID)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for broadcast item %d", i)
		}
	}
}

func TestIngestLogs_UnknownService(t *testing.T) {
	svc, _, _, _ := setupTestService(t)
	ctx := context.Background()

	err := svc.IngestLogs(ctx, "svc-does-not-exist", IngestLogsRequest{
		Logs: []IngestLogItem{{Message: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error for non-existent service, got nil")
	}
}

func TestGetLogs_FilteringAndSearch(t *testing.T) {
	svc, _, _, _ := setupTestService(t)
	ctx := context.Background()

	_ = svc.IngestLogs(ctx, "svc-test-123", IngestLogsRequest{
		DeploymentID: "dep-1",
		Logs: []IngestLogItem{
			{Message: "Worker 1 initialized", Stream: "stdout"},
			{Message: "Worker 2 failed with connection timeout", Stream: "stderr"},
			{Message: "Worker 3 listening on :8080", Stream: "stdout"},
			{Message: "Worker 4 shutting down", Stream: "stdout"},
		},
	})

	// 1. Filter by stream=stderr
	res, err := svc.GetLogs(ctx, "user-1", types.AppLogsQueryFilter{
		ServiceID: "svc-test-123",
		Stream:    "stderr",
	})
	if err != nil {
		t.Fatalf("GetLogs stderr failed: %v", err)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "connection timeout") {
		t.Errorf("unexpected stderr logs: %+v", res.Logs)
	}

	// 2. Filter by search="listening"
	res, err = svc.GetLogs(ctx, "user-1", types.AppLogsQueryFilter{
		ServiceID: "svc-test-123",
		Search:    "listening",
	})
	if err != nil {
		t.Fatalf("GetLogs search failed: %v", err)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "listening on :8080") {
		t.Errorf("unexpected search logs: %+v", res.Logs)
	}

	// 3. Pagination (limit 2, offset 1)
	res, err = svc.GetLogs(ctx, "user-1", types.AppLogsQueryFilter{
		ServiceID: "svc-test-123",
		Limit:     2,
		Offset:    1,
	})
	if err != nil {
		t.Fatalf("GetLogs paginated failed: %v", err)
	}
	if len(res.Logs) != 2 {
		t.Errorf("expected 2 logs for limit=2, got %d", len(res.Logs))
	}
	if res.TotalLines != 4 {
		t.Errorf("expected total 4 lines, got %d", res.TotalLines)
	}
}

func TestGetLogs_Authorization(t *testing.T) {
	svc, _, _, _ := setupTestService(t)
	ctx := context.Background()

	// Authorized user succeeds
	_, err := svc.GetLogs(ctx, "user-1", types.AppLogsQueryFilter{ServiceID: "svc-test-123"})
	if err != nil {
		t.Errorf("expected success for authorized user, got: %v", err)
	}

	// Unauthorized user fails
	_, err = svc.GetLogs(ctx, "user-stranger", types.AppLogsQueryFilter{ServiceID: "svc-test-123"})
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestBroadcaster_PubSub(t *testing.T) {
	broadcaster := NewBroadcaster()
	defer broadcaster.Close()

	ch1, unsub1 := broadcaster.Subscribe("svc-a")
	ch2, unsub2 := broadcaster.Subscribe("svc-a")
	ch3, unsub3 := broadcaster.Subscribe("svc-b")
	defer unsub1()
	defer unsub2()
	defer unsub3()

	if count := broadcaster.ActiveSubscribers("svc-a"); count != 2 {
		t.Errorf("expected 2 active subscribers for svc-a, got %d", count)
	}

	broadcaster.Broadcast(types.AppLogEntry{
		ServiceID: "svc-a",
		Message:   "message for A",
	})

	select {
	case entry := <-ch1:
		if entry.Message != "message for A" {
			t.Errorf("ch1 expected 'message for A', got %s", entry.Message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch1 timed out")
	}

	select {
	case entry := <-ch2:
		if entry.Message != "message for A" {
			t.Errorf("ch2 expected 'message for A', got %s", entry.Message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch2 timed out")
	}

	// ch3 should receive nothing
	select {
	case entry := <-ch3:
		t.Fatalf("ch3 unexpectedly received message: %+v", entry)
	default:
		// success
	}

	// Unsubscribe ch1
	unsub1()
	if count := broadcaster.ActiveSubscribers("svc-a"); count != 1 {
		t.Errorf("expected 1 subscriber after unsub1, got %d", count)
	}
}

func TestSimulatedLogTailer(t *testing.T) {
	tailer := NewSimulatedLogTailer(50*time.Millisecond, false)
	tailer.SetCustomLines([]string{"seed-1", "seed-2"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	outCh := make(chan types.AppLogEntry, 10)
	err := tailer.TailAllocation(ctx, "svc-1", "dep-1", "alloc-1", "server", "stdout", outCh)
	if err != nil {
		t.Fatalf("TailAllocation failed: %v", err)
	}

	if len(outCh) != 2 {
		t.Fatalf("expected 2 seed lines, got %d", len(outCh))
	}

	l1 := <-outCh
	if l1.Message != "seed-1" {
		t.Errorf("expected 'seed-1', got %s", l1.Message)
	}
	l2 := <-outCh
	if l2.Message != "seed-2" {
		t.Errorf("expected 'seed-2', got %s", l2.Message)
	}
}

func TestPurgeLogs(t *testing.T) {
	svc, repo, _, _ := setupTestService(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC()

	_ = repo.InsertLog(ctx, types.AppLogEntry{
		ServiceID: "svc-test-123",
		Message:   "old log line",
		Timestamp: past,
	})
	_ = repo.InsertLog(ctx, types.AppLogEntry{
		ServiceID: "svc-test-123",
		Message:   "recent log line",
		Timestamp: recent,
	})

	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	deleted, err := svc.PurgeLogs(ctx, "user-1", "svc-test-123", cutoff)
	if err != nil {
		t.Fatalf("PurgeLogs failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 log deleted, got %d", deleted)
	}
	if len(repo.logs) != 1 || repo.logs[0].Message != "recent log line" {
		t.Errorf("expected remaining log to be 'recent log line', got %+v", repo.logs)
	}
}

func TestEventSubscribers(t *testing.T) {
	svc, repo, broadcaster, eventBus := setupTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := svc.StartEventSubscribers(ctx)
	if err != nil {
		t.Fatalf("StartEventSubscribers failed: %v", err)
	}

	subCh, unsub := broadcaster.Subscribe("svc-test-123")
	defer unsub()

	// Publish deployment healthy event
	event := events.New(events.SubjectDeployHealthy, "orchestration-service", events.DeployHealthyPayload{
		ServiceID:    "svc-test-123",
		DeploymentID: "dep-live-99",
	})
	_ = eventBus.Publish(ctx, events.SubjectDeployHealthy, event)

	select {
	case entry := <-subCh:
		if entry.Stream != "system" {
			t.Errorf("expected system stream, got %s", entry.Stream)
		}
		if !strings.Contains(entry.Message, "dep-live-99") {
			t.Errorf("expected message to mention deployment, got: %s", entry.Message)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for system event broadcast")
	}

	if len(repo.logs) == 0 {
		t.Fatal("expected system log to be persisted in repo")
	}
}
