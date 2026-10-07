package orchestrator

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

// NomadClientConfig holds configuration for communicating with a live Nomad cluster
type NomadClientConfig struct {
	Address    string        // e.g. "http://127.0.0.1:4646"
	Token      string        // Secret ID / ACL token
	Datacenter string        // e.g. "dc1"
	Namespace  string        // e.g. "default"
	Timeout    time.Duration // HTTP client timeout
}

// NomadHTTPClient interacts with the official HashiCorp Nomad HTTP REST API
type NomadHTTPClient struct {
	address    string
	token      string
	namespace  string
	datacenter string
	httpClient *http.Client
}

// NewNomadHTTPClient creates a live Nomad API client
func NewNomadHTTPClient(cfg NomadClientConfig) *NomadHTTPClient {
	addr := strings.TrimRight(cfg.Address, "/")
	if addr == "" {
		addr = "http://127.0.0.1:4646"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}

	return &NomadHTTPClient{
		address:    addr,
		token:      cfg.Token,
		namespace:  ns,
		datacenter: cfg.Datacenter,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SubmitJob parses HCL2 job spec and submits to Nomad API
func (c *NomadHTTPClient) SubmitJob(ctx context.Context, jobSpecHCL string) (*JobSubmissionResult, error) {
	// 1. Parse HCL spec to Nomad JSON schema via /v1/jobs/parse
	parsedJob, err := c.parseJobHCL(ctx, jobSpecHCL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job HCL with Nomad: %w", err)
	}

	// 2. Register job via POST /v1/jobs
	payload := map[string]any{
		"Job": parsedJob,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/v1/jobs", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nomad register error (%d): %s", resp.StatusCode, string(b))
	}

	var res struct {
		EvalID          string   `json:"EvalID"`
		EvalCreateIndex uint64   `json:"EvalCreateIndex"`
		JobModifyIndex  uint64   `json:"JobModifyIndex"`
		KnownLeader     bool     `json:"KnownLeader"`
		LastIndex       uint64   `json:"LastIndex"`
		Warnings        string   `json:"Warnings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	jobID := ""
	if idVal, ok := parsedJob["ID"].(string); ok {
		jobID = idVal
	}

	var warnings []string
	if res.Warnings != "" {
		warnings = strings.Split(res.Warnings, "\n")
	}

	return &JobSubmissionResult{
		JobID:           jobID,
		EvalID:          res.EvalID,
		EvalCreateIndex: res.EvalCreateIndex,
		JobModifyIndex:  res.JobModifyIndex,
		KnownLeader:     res.KnownLeader,
		LastIndex:       res.LastIndex,
		Warnings:        warnings,
	}, nil
}

// GetJob reads job status from Nomad API
func (c *NomadHTTPClient) GetJob(ctx context.Context, jobID string) (*JobStatus, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/job/%s", jobID), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrJobNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nomad get job error (%d): %s", resp.StatusCode, string(b))
	}

	var raw struct {
		ID          string `json:"ID"`
		Status      string `json:"Status"`
		StatusDesc  string `json:"StatusDescription"`
		Type        string `json:"Type"`
		Version     uint64 `json:"Version"`
		ModifyIndex uint64 `json:"ModifyIndex"`
		Meta        map[string]string `json:"Meta"`
		SubmitTime  int64  `json:"SubmitTime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	activeDeploy := ""
	if raw.Meta != nil {
		activeDeploy = raw.Meta["adroit.deploy_id"]
	}

	return &JobStatus{
		JobID:            raw.ID,
		Status:           raw.Status,
		StatusDescription: raw.StatusDesc,
		Type:             raw.Type,
		Version:          raw.Version,
		ModifyIndex:      raw.ModifyIndex,
		ActiveDeployment: activeDeploy,
		CreatedAt:        time.Unix(0, raw.SubmitTime).UTC(),
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

// GetAllocations returns allocations for a job
func (c *NomadHTTPClient) GetAllocations(ctx context.Context, jobID string) ([]AllocationInfo, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/job/%s/allocations", jobID), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrJobNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nomad allocations error (%d): %s", resp.StatusCode, string(b))
	}

	var rawAllocs []struct {
		ID            string `json:"ID"`
		JobID         string `json:"JobID"`
		NodeID        string `json:"NodeID"`
		NodeName      string `json:"NodeName"`
		ClientStatus  string `json:"ClientStatus"`
		DesiredStatus string `json:"DesiredStatus"`
		ModifyIndex   uint64 `json:"ModifyIndex"`
		CreateTime    int64  `json:"CreateTime"`
		ModifyTime    int64  `json:"ModifyTime"`
		DeploymentStatus *struct {
			Healthy *bool `json:"Healthy"`
			Canary  bool  `json:"Canary"`
		} `json:"DeploymentStatus"`
		TaskStates map[string]struct {
			State      string `json:"State"`
			Failed     bool   `json:"Failed"`
			Restarts   int    `json:"Restarts"`
		} `json:"TaskStates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawAllocs); err != nil {
		return nil, err
	}

	allocs := make([]AllocationInfo, len(rawAllocs))
	for i, a := range rawAllocs {
		healthy := false
		isCanary := false
		if a.DeploymentStatus != nil {
			if a.DeploymentStatus.Healthy != nil {
				healthy = *a.DeploymentStatus.Healthy
			}
			isCanary = a.DeploymentStatus.Canary
		}

		taskStates := make(map[string]TaskState)
		for k, v := range a.TaskStates {
			taskStates[k] = TaskState{
				State:    v.State,
				Failed:   v.Failed,
				Restarts: v.Restarts,
			}
		}

		allocs[i] = AllocationInfo{
			ID:            a.ID,
			JobID:         a.JobID,
			NodeID:        a.NodeID,
			NodeName:      a.NodeName,
			ClientStatus:  a.ClientStatus,
			DesiredStatus: a.DesiredStatus,
			Healthy:       healthy,
			IsCanary:      isCanary,
			TaskStates:    taskStates,
			ModifyIndex:   a.ModifyIndex,
			CreatedAt:     time.Unix(0, a.CreateTime).UTC(),
			UpdatedAt:     time.Unix(0, a.ModifyTime).UTC(),
		}
	}

	return allocs, nil
}

// PromoteCanary promotes canary deployment
func (c *NomadHTTPClient) PromoteCanary(ctx context.Context, jobID string) error {
	// Look up active deployment ID for the job
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/job/%s/deployments", jobID), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrJobNotFound
	}

	var deployments []struct {
		ID     string `json:"ID"`
		Status string `json:"Status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return err
	}

	if len(deployments) == 0 {
		return fmt.Errorf("no active nomad deployment found for job %s", jobID)
	}

	activeDeploymentID := deployments[0].ID

	// Promote canary via POST /v1/deployment/promote/:id
	promotePayload := map[string]any{
		"DeploymentID": activeDeploymentID,
		"All":          true,
	}
	body, _ := json.Marshal(promotePayload)

	promoteReq, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/v1/deployment/promote/%s", activeDeploymentID), bytes.NewReader(body))
	if err != nil {
		return err
	}

	promoteResp, err := c.httpClient.Do(promoteReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer promoteResp.Body.Close()

	if promoteResp.StatusCode < 200 || promoteResp.StatusCode >= 300 {
		b, _ := io.ReadAll(promoteResp.Body)
		return fmt.Errorf("nomad promote error (%d): %s", promoteResp.StatusCode, string(b))
	}

	return nil
}

// ScaleJob modifies the replica count of the job's app group
func (c *NomadHTTPClient) ScaleJob(ctx context.Context, jobID string, count int) error {
	scalePayload := map[string]any{
		"Count":  count,
		"Target": map[string]string{"Group": "app"},
	}
	body, _ := json.Marshal(scalePayload)

	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/v1/job/%s/scale", jobID), bytes.NewReader(body))
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrJobNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nomad scale error (%d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// StopJob deregisters or stops a Nomad job
func (c *NomadHTTPClient) StopJob(ctx context.Context, jobID string, purge bool) error {
	endpoint := fmt.Sprintf("/v1/job/%s?purge=%t", jobID, purge)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrJobNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nomad stop error (%d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// CheckHealth inspects allocations and deployment status
func (c *NomadHTTPClient) CheckHealth(ctx context.Context, jobID string) (*HealthResult, error) {
	allocs, err := c.GetAllocations(ctx, jobID)
	if err != nil {
		return nil, err
	}

	total := 0
	passing := 0
	failing := 0
	canaryReady := false

	for _, a := range allocs {
		// Ignore dead, completed, or stopped allocations from previous revisions
		if a.DesiredStatus == "stop" || a.ClientStatus == "complete" || a.ClientStatus == "lost" || a.ClientStatus == "failed" {
			continue
		}
		total++
		if a.ClientStatus == "running" && a.Healthy {
			passing++
			if a.IsCanary {
				canaryReady = true
			}
		} else {
			failing++
		}
	}

	healthy := (passing == total && total > 0)
	return &HealthResult{
		Healthy:      healthy,
		TotalCount:   total,
		PassingCount: passing,
		FailingCount: failing,
		CanaryReady:  canaryReady,
		Message:      fmt.Sprintf("%d/%d allocations healthy", passing, total),
	}, nil
}

func (c *NomadHTTPClient) parseJobHCL(ctx context.Context, hclSpec string) (map[string]any, error) {
	payload := map[string]any{
		"JobHCL":    hclSpec,
		"Canonical": true,
	}
	body, _ := json.Marshal(payload)

	req, err := c.newRequest(ctx, http.MethodPost, "/v1/jobs/parse", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDriverUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nomad hcl parse error (%d): %s", resp.StatusCode, string(b))
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}

func (c *NomadHTTPClient) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.address, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Nomad-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Nomad-Namespace", c.namespace)
	}

	return req, nil
}
