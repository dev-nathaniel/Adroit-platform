package client

import (
	"context"
	"fmt"
	"io"
	"net/url"
)

// ============================================================================
// AUTH
// ============================================================================

func (c *Client) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	req := map[string]string{
		"email":    email,
		"password": password,
	}
	var res AuthResponse
	if err := c.Post(ctx, "/api/v1/auth/login", req, &res); err != nil {
		return nil, err
	}
	c.Token = res.Token
	return &res, nil
}

func (c *Client) Register(ctx context.Context, email, password, name, orgName string) (*AuthResponse, error) {
	req := map[string]string{
		"email":    email,
		"password": password,
		"name":     name,
		"org_name": orgName,
	}
	var res AuthResponse
	if err := c.Post(ctx, "/api/v1/auth/register", req, &res); err != nil {
		return nil, err
	}
	c.Token = res.Token
	return &res, nil
}

func (c *Client) Me(ctx context.Context) (*User, error) {
	var res User
	if err := c.Get(ctx, "/api/v1/auth/me", &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ============================================================================
// PROJECTS
// ============================================================================

func (c *Client) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	path := "/api/v1/projects"
	if orgID != "" {
		path += "?organization_id=" + url.QueryEscape(orgID)
	}
	var res []Project
	if err := c.Get(ctx, path, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) CreateProject(ctx context.Context, orgID, name, description string) (*Project, error) {
	req := map[string]string{
		"organization_id": orgID,
		"name":            name,
		"description":     description,
	}
	var res Project
	if err := c.Post(ctx, "/api/v1/projects", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var res Project
	if err := c.Get(ctx, "/api/v1/projects/"+projectID, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	return c.Delete(ctx, "/api/v1/projects/"+projectID, nil)
}

// ============================================================================
// SERVICES
// ============================================================================

func (c *Client) ListServices(ctx context.Context, orgID, projectID string) ([]Service, error) {
	params := url.Values{}
	if orgID != "" {
		params.Set("organization_id", orgID)
	}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	path := "/api/v1/services"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var res []Service
	if err := c.Get(ctx, path, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) CreateService(ctx context.Context, req map[string]any) (*Service, error) {
	var res Service
	if err := c.Post(ctx, "/api/v1/services", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetService(ctx context.Context, serviceID string) (*Service, error) {
	var res Service
	if err := c.Get(ctx, "/api/v1/services/"+serviceID, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) DeleteService(ctx context.Context, serviceID string) error {
	return c.Delete(ctx, "/api/v1/services/"+serviceID, nil)
}

func (c *Client) UploadSource(ctx context.Context, serviceID string, contentType string, reader io.Reader, triggerBuild bool) (*UploadResponse, error) {
	path := fmt.Sprintf("/api/v1/services/%s/upload?trigger_build=%t", serviceID, triggerBuild)
	var res UploadResponse
	if err := c.PostRaw(ctx, path, contentType, reader, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ============================================================================
// DEPLOYMENTS & ORCHESTRATION
// ============================================================================

func (c *Client) Deploy(ctx context.Context, serviceID string, req map[string]any) (*Deployment, error) {
	var res Deployment
	if err := c.Post(ctx, fmt.Sprintf("/api/v1/services/%s/deploy", serviceID), req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	var res Deployment
	if err := c.Get(ctx, "/api/v1/deployments/"+deploymentID, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListDeployments(ctx context.Context, serviceID string) ([]Deployment, error) {
	var res []Deployment
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/services/%s/deployments", serviceID), &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) ScaleService(ctx context.Context, serviceID string, replicas int) error {
	req := map[string]int{"replicas": replicas}
	return c.Post(ctx, fmt.Sprintf("/api/v1/services/%s/scale", serviceID), req, nil)
}

func (c *Client) RollbackDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	var res Deployment
	if err := c.Post(ctx, fmt.Sprintf("/api/v1/deployments/%s/rollback", deploymentID), nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) PromoteDeployment(ctx context.Context, deploymentID string) error {
	return c.Post(ctx, fmt.Sprintf("/api/v1/deployments/%s/promote", deploymentID), nil, nil)
}

func (c *Client) GetAllocations(ctx context.Context, deploymentID string) ([]Allocation, error) {
	var res []Allocation
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/deployments/%s/allocations", deploymentID), &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ============================================================================
// BUILDS
// ============================================================================

func (c *Client) ListBuilds(ctx context.Context, serviceID string) ([]Build, error) {
	var res []Build
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/services/%s/builds", serviceID), &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) GetBuild(ctx context.Context, buildID string) (*Build, error) {
	var res Build
	if err := c.Get(ctx, "/api/v1/builds/"+buildID, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetBuildLogs(ctx context.Context, buildID string) (string, error) {
	var res struct {
		Logs string `json:"logs"`
	}
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/builds/%s/logs", buildID), &res); err != nil {
		return "", err
	}
	return res.Logs, nil
}

// ============================================================================
// ENVIRONMENT VARIABLES & SECRETS
// ============================================================================

func (c *Client) GetServiceEnv(ctx context.Context, serviceID string) ([]EnvVar, error) {
	var res []EnvVar
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/services/%s/env", serviceID), &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) SetServiceEnv(ctx context.Context, serviceID string, variables []EnvVar) ([]EnvVar, error) {
	req := map[string]any{"variables": variables}
	var res []EnvVar
	if err := c.Post(ctx, fmt.Sprintf("/api/v1/services/%s/env", serviceID), req, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) DeleteServiceEnv(ctx context.Context, serviceID, key string) error {
	return c.Delete(ctx, fmt.Sprintf("/api/v1/services/%s/env/%s", serviceID, key), nil)
}

func (c *Client) RevealServiceSecret(ctx context.Context, serviceID, key string) (string, error) {
	var res struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.Post(ctx, fmt.Sprintf("/api/v1/services/%s/env/%s/reveal", serviceID, key), nil, &res); err != nil {
		return "", err
	}
	return res.Value, nil
}

// ============================================================================
// CUSTOM DOMAINS
// ============================================================================

func (c *Client) ListDomains(ctx context.Context, serviceID string) ([]CustomDomain, error) {
	var res []CustomDomain
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/services/%s/domains", serviceID), &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) AddDomain(ctx context.Context, serviceID, domain string) (*CustomDomain, error) {
	req := map[string]string{"domain": domain}
	var res CustomDomain
	if err := c.Post(ctx, fmt.Sprintf("/api/v1/services/%s/domains", serviceID), req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) VerifyDomain(ctx context.Context, domainID string) error {
	return c.Post(ctx, fmt.Sprintf("/api/v1/domains/%s/verify", domainID), nil, nil)
}

func (c *Client) DeleteDomain(ctx context.Context, domainID string) error {
	return c.Delete(ctx, fmt.Sprintf("/api/v1/domains/%s", domainID), nil)
}
