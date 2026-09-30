package logservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// IngestLogItem represents a single log line within an ingestion batch
type IngestLogItem struct {
	Message      string     `json:"message"`
	Stream       string     `json:"stream,omitempty"` // "stdout", "stderr", "system"; defaults to "stdout"
	Timestamp    *time.Time `json:"timestamp,omitempty"`
	DeploymentID *string    `json:"deployment_id,omitempty"`
	AllocationID *string    `json:"allocation_id,omitempty"`
}

// IngestLogsRequest payload for submitting batch log records
type IngestLogsRequest struct {
	ServiceID    string          `json:"service_id,omitempty"`
	DeploymentID string          `json:"deployment_id,omitempty"`
	AllocationID string          `json:"allocation_id,omitempty"`
	Logs         []IngestLogItem `json:"logs"`
}

// LogStreamParams defines query options for live streaming (SSE / WebSocket)
type LogStreamParams struct {
	ServiceID    string `json:"service_id"`
	DeploymentID string `json:"deployment_id,omitempty"`
	AllocationID string `json:"allocation_id,omitempty"`
	Stream       string `json:"stream,omitempty"` // "stdout", "stderr", or "all"
	Search       string `json:"search,omitempty"` // Substring search filter
	Tail         int    `json:"tail,omitempty"`   // Pre-fetch recent N lines from DB before live streaming (default 50)
	Follow       bool   `json:"follow,omitempty"` // Whether to keep stream open (default true)
}

// WebSocketClientMessage encapsulates incoming control messages from WebSocket clients
type WebSocketClientMessage struct {
	Action string `json:"action"` // "ping", "filter"
	Filter string `json:"filter,omitempty"`
}

// WebSocketServerMessage represents an outgoing frame over WebSocket
type WebSocketServerMessage struct {
	Type string            `json:"type"` // "log", "pong", "connected", "error"
	Log  *types.AppLogEntry `json:"log,omitempty"`
	Text string            `json:"text,omitempty"`
}
