package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LogEntry represents an individual log line emitted by a workload container.
type LogEntry struct {
	ID           string    `json:"id,omitempty"`
	ServiceID    string    `json:"service_id"`
	DeploymentID string    `json:"deployment_id,omitempty"`
	InstanceID   string    `json:"instance_id,omitempty"`
	Stream       string    `json:"stream"` // "stdout" or "stderr"
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// LogStreamOptions configures log filtering and tailing behavior.
type LogStreamOptions struct {
	Tail   int
	Stream string
	Search string
}

// StreamLogs connects to the SSE log endpoint and streams events to the callback handler.
func (c *Client) StreamLogs(ctx context.Context, serviceID string, opts LogStreamOptions, handler func(entry LogEntry)) error {
	params := url.Values{}
	if opts.Tail > 0 {
		params.Set("tail", strconv.Itoa(opts.Tail))
	}
	if opts.Stream != "" {
		params.Set("stream", opts.Stream)
	}
	if opts.Search != "" {
		params.Set("search", opts.Search)
	}

	endpoint := fmt.Sprintf("%s/api/v1/services/%s/logs/stream?%s", c.BaseURL, serviceID, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create log stream request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Disable client timeout for long-lived stream
	streamClient := &http.Client{
		Timeout: 0,
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to open log stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("log stream returned HTTP status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Ignore comment lines (e.g. ": keep-alive") and blank lines
		if strings.HasPrefix(line, ":") || strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataPayload := strings.TrimPrefix(line, "data: ")
			var entry LogEntry
			if err := json.Unmarshal([]byte(dataPayload), &entry); err == nil {
				handler(entry)
			}
		}
	}

	return scanner.Err()
}
