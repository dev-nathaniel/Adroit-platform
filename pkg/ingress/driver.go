package ingress

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRouteNotFound     = errors.New("ingress route not found")
	ErrInvalidHostPattern = errors.New("invalid host pattern")
	ErrNoUpstreams       = errors.New("at least one upstream dial target is required")
	ErrCaddyUnavailable  = errors.New("caddy admin api is unreachable")
)

// UpstreamTarget specifies a network address to dial for reverse proxying
type UpstreamTarget struct {
	Dial string `json:"dial"` // e.g. "127.0.0.1:8080" or "alloc-node-1:24001"
}

// RouteSpec defines the parameters needed to configure an edge reverse proxy route
type RouteSpec struct {
	ID          string           `json:"id"`           // e.g. "route_api_customer_com"
	HostPattern string           `json:"host_pattern"` // e.g. "api.customer.com" or "*.adroit.run"
	Upstreams   []UpstreamTarget `json:"upstreams"`
	EnableTLS   bool             `json:"enable_tls"`
}

// RouteInfo represents an active edge route retrieved from the ingress driver
type RouteInfo struct {
	ID          string           `json:"id"`
	HostPattern string           `json:"host_pattern"`
	Upstreams   []UpstreamTarget `json:"upstreams"`
	Active      bool             `json:"active"`
	SyncedAt    time.Time        `json:"synced_at"`
}

// Driver defines the contract for synchronizing edge ingress reverse proxy routes
type Driver interface {
	// UpsertRoute creates or replaces an edge routing rule for a host pattern
	UpsertRoute(ctx context.Context, spec RouteSpec) error

	// DeleteRoute removes an edge routing rule by route ID or host pattern
	DeleteRoute(ctx context.Context, routeID string) error

	// GetRoute retrieves active route information by route ID
	GetRoute(ctx context.Context, routeID string) (*RouteInfo, error)

	// ListRoutes returns all active edge routes currently configured in the ingress proxy
	ListRoutes(ctx context.Context) ([]RouteInfo, error)

	// CheckHealth verifies connectivity and readiness of the ingress controller
	CheckHealth(ctx context.Context) error
}
