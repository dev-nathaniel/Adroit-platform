package logservice

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// LogTailer defines the contract for streaming logs from runtime allocation instances
type LogTailer interface {
	TailAllocation(ctx context.Context, serviceID, deploymentID, allocID, task, stream string, out chan<- types.AppLogEntry) error
}

// NomadTailerConfig configuration for connecting to Nomad's filesystem log API
type NomadTailerConfig struct {
	NomadAddress string
	NomadToken   string
	HTTPClient   *http.Client
}

// NomadLogTailer connects to Nomad client agents and tails container stdout/stderr
type NomadLogTailer struct {
	address    string
	token      string
	httpClient *http.Client
}

// NewNomadLogTailer creates a new NomadLogTailer
func NewNomadLogTailer(cfg NomadTailerConfig) *NomadLogTailer {
	addr := strings.TrimRight(cfg.NomadAddress, "/")
	if addr == "" {
		addr = "http://127.0.0.1:4646"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 0, // Streaming connection without total timeout
		}
	}

	return &NomadLogTailer{
		address:    addr,
		token:      cfg.NomadToken,
		httpClient: client,
	}
}

// TailAllocation streams live log lines from Nomad's client log API
func (t *NomadLogTailer) TailAllocation(ctx context.Context, serviceID, deploymentID, allocID, task, stream string, out chan<- types.AppLogEntry) error {
	if task == "" {
		task = "server"
	}
	if stream == "" {
		stream = "stdout"
	}

	reqURL := fmt.Sprintf("%s/v1/client/fs/logs/%s?task=%s&type=%s&follow=true&origin=end", t.address, allocID, task, stream)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create Nomad log request: %w", err)
	}

	if t.token != "" {
		req.Header.Set("X-Nomad-Token", t.token)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stream logs from Nomad client: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nomad log stream error (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		entry := types.AppLogEntry{
			ID:           newLogID(),
			ServiceID:    serviceID,
			DeploymentID: strPtr(deploymentID),
			AllocationID: strPtr(allocID),
			Stream:       stream,
			Message:      line,
			Timestamp:    time.Now().UTC(),
		}

		select {
		case out <- entry:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return scanner.Err()
}

// SimulatedLogTailer provides synthetic application logs for local dev and testing
type SimulatedLogTailer struct {
	mu           sync.Mutex
	interval     time.Duration
	customLines  []string
	autoGenerate bool
}

// NewSimulatedLogTailer creates an in-memory simulated log generator
func NewSimulatedLogTailer(interval time.Duration, autoGenerate bool) *SimulatedLogTailer {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &SimulatedLogTailer{
		interval:     interval,
		autoGenerate: autoGenerate,
		customLines: []string{
			`[info] Runtime initialized (Go 1.27.1 / amd64)`,
			`[info] Loading environment variables from Nomad job metadata`,
			`[info] HTTP server listening on 0.0.0.0:8080`,
			`[info] Consul Connect service registration confirmed: status=passing`,
			`[info] GET /healthz 200 OK 0.8ms (Consul Health Check)`,
			`[info] GET /api/v1/ping 200 OK 2.1ms (Edge Ingress Routing)`,
			`[info] Worker pool started with 8 concurrent workers`,
		},
	}
}

// SetCustomLines sets predefined log messages for the simulator
func (s *SimulatedLogTailer) SetCustomLines(lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.customLines = lines
}

// TailAllocation streams predefined and periodic simulated log messages
func (s *SimulatedLogTailer) TailAllocation(ctx context.Context, serviceID, deploymentID, allocID, task, stream string, out chan<- types.AppLogEntry) error {
	if stream == "" {
		stream = "stdout"
	}

	s.mu.Lock()
	lines := append([]string(nil), s.customLines...)
	s.mu.Unlock()

	// 1. Emit seed lines first
	for _, line := range lines {
		entry := types.AppLogEntry{
			ID:           newLogID(),
			ServiceID:    serviceID,
			DeploymentID: strPtr(deploymentID),
			AllocationID: strPtr(allocID),
			Stream:       stream,
			Message:      line,
			Timestamp:    time.Now().UTC(),
		}

		select {
		case out <- entry:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if !s.autoGenerate {
		return nil
	}

	// 2. Continuously generate simulated access logs on ticker
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	counter := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-ticker.C:
			msg := fmt.Sprintf(`[info] GET /api/v1/items?page=%d 200 OK 1.%dms - handled incoming request`, counter, counter%9)
			if counter%5 == 0 {
				msg = fmt.Sprintf(`[warn] Slow response detected on /api/v1/reports: 124ms (threshold: 100ms)`)
			}

			entry := types.AppLogEntry{
				ID:           newLogID(),
				ServiceID:    serviceID,
				DeploymentID: strPtr(deploymentID),
				AllocationID: strPtr(allocID),
				Stream:       stream,
				Message:      msg,
				Timestamp:    t.UTC(),
			}

			select {
			case out <- entry:
			case <-ctx.Done():
				return ctx.Err()
			}
			counter++
		}
	}
}

func newLogID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "log_" + hex.EncodeToString(b)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
