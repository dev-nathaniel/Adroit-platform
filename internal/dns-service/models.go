package dnsservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// CreateDomainRequest is the payload submitted to add a custom domain to a service
type CreateDomainRequest struct {
	Domain           string                 `json:"domain"`
	VerificationType types.VerificationType `json:"verification_type,omitempty"` // "cname" (default) or "txt"
	IsPrimary        bool                   `json:"is_primary,omitempty"`
}

// DomainResponse represents a custom domain with full challenge instructions
type DomainResponse struct {
	Domain           types.CustomDomain        `json:"domain"`
	ChallengeDetails types.DNSChallengeDetails `json:"challenge_details"`
}

// VerifyDomainResponse describes the outcome of a manual or automatic verification check
type VerifyDomainResponse struct {
	Verified         bool                      `json:"verified"`
	Status           types.DomainStatus        `json:"status"`
	Message          string                    `json:"message"`
	Domain           types.CustomDomain        `json:"domain"`
	ChallengeDetails types.DNSChallengeDetails `json:"challenge_details"`
}

// SyncRouteRequest allows manual re-synchronization of edge ingress proxying
type SyncRouteRequest struct {
	UpstreamURL string `json:"upstream_url,omitempty"` // Optional manual upstream override (e.g. "127.0.0.1:8080")
}

// IngressRouteResponse wraps an active edge route
type IngressRouteResponse struct {
	ID           string    `json:"id"`
	ServiceID    string    `json:"service_id"`
	DomainID     *string   `json:"domain_id,omitempty"`
	HostPattern  string    `json:"host_pattern"`
	UpstreamURL  string    `json:"upstream_url"`
	IsActive     bool      `json:"is_active"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

// ServiceRouteSyncResult summarizes dynamic Caddy routes updated for a deployment
type ServiceRouteSyncResult struct {
	ServiceID    string   `json:"service_id"`
	UpstreamURL  string   `json:"upstream_url"`
	SyncedHosts  []string `json:"synced_hosts"`
	FailedHosts  []string `json:"failed_hosts,omitempty"`
	LastSyncedAt time.Time`json:"last_synced_at"`
}
