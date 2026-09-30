package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CaddyRouteConfig represents Caddy's internal JSON route representation
type CaddyRouteConfig struct {
	ID       string              `json:"@id,omitempty"`
	Match    []CaddyMatchConfig  `json:"match,omitempty"`
	Handle   []CaddyHandleConfig `json:"handle,omitempty"`
	Terminal bool                `json:"terminal"`
}

type CaddyMatchConfig struct {
	Host []string `json:"host,omitempty"`
}

type CaddyHandleConfig struct {
	Handler   string           `json:"handler"`
	Upstreams []UpstreamTarget `json:"upstreams,omitempty"`
}

// CaddyClient communicates directly with Caddy's dynamic JSON Admin API
type CaddyClient struct {
	adminURL   string
	serverName string
	httpClient *http.Client
}

// NewCaddyClient initializes an HTTP client for Caddy Admin API
func NewCaddyClient(adminURL string, serverName string) *CaddyClient {
	if adminURL == "" {
		adminURL = "http://localhost:2019"
	}
	if serverName == "" {
		serverName = "adroit_edge"
	}
	adminURL = strings.TrimRight(adminURL, "/")

	return &CaddyClient{
		adminURL:   adminURL,
		serverName: serverName,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// EnsureServer guarantees that the target HTTP server block exists in Caddy
func (c *CaddyClient) EnsureServer(ctx context.Context) error {
	url := fmt.Sprintf("%s/config/apps/http/servers/%s", c.adminURL, c.serverName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Server doesn't exist, create it with standard HTTP/HTTPS listener
	serverPayload := map[string]any{
		"listen": []string{":80", ":443"},
		"routes": []any{},
	}
	body, err := json.Marshal(serverPayload)
	if err != nil {
		return err
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := c.httpClient.Do(putReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("failed to bootstrap caddy server block: %s", string(respBody))
	}

	return nil
}

// UpsertRoute registers or updates a route via Caddy's @id mechanism
func (c *CaddyClient) UpsertRoute(ctx context.Context, spec RouteSpec) error {
	if spec.HostPattern == "" {
		return ErrInvalidHostPattern
	}
	if len(spec.Upstreams) == 0 {
		return ErrNoUpstreams
	}
	if spec.ID == "" {
		spec.ID = GenerateRouteID(spec.HostPattern)
	}

	caddyRoute := CaddyRouteConfig{
		ID: spec.ID,
		Match: []CaddyMatchConfig{
			{Host: []string{spec.HostPattern}},
		},
		Handle: []CaddyHandleConfig{
			{
				Handler:   "reverse_proxy",
				Upstreams: spec.Upstreams,
			},
		},
		Terminal: true,
	}

	data, err := json.Marshal(caddyRoute)
	if err != nil {
		return err
	}

	// 1. Try replacing via /id/<route_id>
	idURL := fmt.Sprintf("%s/id/%s", c.adminURL, spec.ID)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, idURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := c.httpClient.Do(putReq)
	if err == nil {
		defer putResp.Body.Close()
		if putResp.StatusCode >= 200 && putResp.StatusCode < 300 {
			return nil
		}
	}

	// 2. If not found by ID or update failed, ensure server exists and append route
	if err := c.EnsureServer(ctx); err != nil {
		return err
	}

	routesURL := fmt.Sprintf("%s/config/apps/http/servers/%s/routes", c.adminURL, c.serverName)
	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, routesURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := c.httpClient.Do(postReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(postResp.Body)
		return fmt.Errorf("caddy route insertion error (status %d): %s", postResp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteRoute removes an edge routing rule from Caddy by its @id
func (c *CaddyClient) DeleteRoute(ctx context.Context, routeID string) error {
	if routeID == "" {
		return ErrRouteNotFound
	}
	url := fmt.Sprintf("%s/id/%s", c.adminURL, routeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrRouteNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy route deletion error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetRoute fetches a route by its @id
func (c *CaddyClient) GetRoute(ctx context.Context, routeID string) (*RouteInfo, error) {
	url := fmt.Sprintf("%s/id/%s", c.adminURL, routeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRouteNotFound
	}

	var cr CaddyRouteConfig
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}

	hostPattern := ""
	if len(cr.Match) > 0 && len(cr.Match[0].Host) > 0 {
		hostPattern = cr.Match[0].Host[0]
	}

	var upstreams []UpstreamTarget
	for _, h := range cr.Handle {
		if h.Handler == "reverse_proxy" {
			upstreams = append(upstreams, h.Upstreams...)
		}
	}

	return &RouteInfo{
		ID:          cr.ID,
		HostPattern: hostPattern,
		Upstreams:   upstreams,
		Active:      true,
		SyncedAt:    time.Now().UTC(),
	}, nil
}

// ListRoutes enumerates all routes configured on the Caddy server block
func (c *CaddyClient) ListRoutes(ctx context.Context) ([]RouteInfo, error) {
	url := fmt.Sprintf("%s/config/apps/http/servers/%s/routes", c.adminURL, c.serverName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []RouteInfo{}, nil
	}

	var rawRoutes []CaddyRouteConfig
	if err := json.NewDecoder(resp.Body).Decode(&rawRoutes); err != nil {
		return nil, err
	}

	var results []RouteInfo
	for _, r := range rawRoutes {
		hostPattern := ""
		if len(r.Match) > 0 && len(r.Match[0].Host) > 0 {
			hostPattern = r.Match[0].Host[0]
		}
		var upstreams []UpstreamTarget
		for _, h := range r.Handle {
			if h.Handler == "reverse_proxy" {
				upstreams = append(upstreams, h.Upstreams...)
			}
		}

		results = append(results, RouteInfo{
			ID:          r.ID,
			HostPattern: hostPattern,
			Upstreams:   upstreams,
			Active:      true,
			SyncedAt:    time.Now().UTC(),
		})
	}

	return results, nil
}

// CheckHealth probes Caddy's /config/ endpoint
func (c *CaddyClient) CheckHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.adminURL+"/config/", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaddyUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caddy unhealthy status %d", resp.StatusCode)
	}
	return nil
}

// GenerateRouteID creates a canonical route ID from a domain or host pattern
func GenerateRouteID(hostPattern string) string {
	clean := strings.ToLower(strings.TrimSpace(hostPattern))
	clean = strings.ReplaceAll(clean, "*.", "wildcard_")
	clean = strings.ReplaceAll(clean, ".", "_")
	clean = strings.ReplaceAll(clean, ":", "_")
	return "route_" + clean
}
