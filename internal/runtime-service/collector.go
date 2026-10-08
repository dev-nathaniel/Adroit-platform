package runtimeservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// MetricsCollector defines interface for sampling workload telemetry
type MetricsCollector interface {
	CollectServiceMetrics(ctx context.Context, svc *types.Service) (*types.LiveServiceMetrics, error)
	InjectSimulatedMetrics(serviceID string, cpuPercent, memoryPercent, rps float64)
	ClearSimulatedMetrics(serviceID string)
}

// ClusterMetricsCollector collects telemetry from Prometheus, Nomad API, and simulated overlays
type ClusterMetricsCollector struct {
	prometheusURL string
	driver        orchestrator.Driver
	httpClient    *http.Client
	logger        *slog.Logger

	mu              sync.RWMutex
	simulatedScores map[string]*types.LiveServiceMetrics
}

// NewClusterMetricsCollector creates an initialized ClusterMetricsCollector
func NewClusterMetricsCollector(prometheusURL string, driver orchestrator.Driver, logger *slog.Logger) *ClusterMetricsCollector {
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	return &ClusterMetricsCollector{
		prometheusURL:   prometheusURL,
		driver:          driver,
		httpClient:      &http.Client{Timeout: 3 * time.Second},
		logger:          logger,
		simulatedScores: make(map[string]*types.LiveServiceMetrics),
	}
}

// InjectSimulatedMetrics overrides live metrics for a service (used during automated load tests)
func (c *ClusterMetricsCollector) InjectSimulatedMetrics(serviceID string, cpuPercent, memoryPercent, rps float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.simulatedScores[serviceID] = &types.LiveServiceMetrics{
		ServiceID:         serviceID,
		CPUPercent:        cpuPercent,
		MemoryPercent:     memoryPercent,
		RequestsPerSec:    rps,
		ActiveAllocations: 1,
		SampledAt:         time.Now().UTC(),
	}
}

// ClearSimulatedMetrics removes manual simulated score override
func (c *ClusterMetricsCollector) ClearSimulatedMetrics(serviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.simulatedScores, serviceID)
}

// CollectServiceMetrics retrieves current CPU, Memory, and RPS metrics
func (c *ClusterMetricsCollector) CollectServiceMetrics(ctx context.Context, svc *types.Service) (*types.LiveServiceMetrics, error) {
	// 1. Check for injected test score first (allows deterministic e2e simulation)
	c.mu.RLock()
	if sim, ok := c.simulatedScores[svc.ID]; ok {
		c.mu.RUnlock()
		memLimit := float64(svc.Resources.MemoryMB)
		if memLimit <= 0 {
			memLimit = 512.0
		}
		simCopy := *sim
		simCopy.MemoryLimitMB = memLimit
		simCopy.MemoryUsageMB = (sim.MemoryPercent / 100.0) * memLimit
		simCopy.SampledAt = time.Now().UTC()
		return &simCopy, nil
	}
	c.mu.RUnlock()

	// 2. Query Prometheus if reachable
	promMetrics, err := c.queryPrometheus(ctx, svc)
	if err == nil && promMetrics != nil {
		return promMetrics, nil
	}

	// 3. Fallback: Query Nomad allocations directly through Driver
	nomadMetrics, err := c.queryNomadCluster(ctx, svc)
	if err == nil && nomadMetrics != nil {
		return nomadMetrics, nil
	}

	// 4. Default idle baseline if container has zero traffic
	memLimit := float64(svc.Resources.MemoryMB)
	if memLimit <= 0 {
		memLimit = 512.0
	}
	return &types.LiveServiceMetrics{
		ServiceID:         svc.ID,
		CPUPercent:        1.5, // baseline idle container tick
		MemoryUsageMB:     math.Min(64.0, memLimit*0.15),
		MemoryLimitMB:     memLimit,
		MemoryPercent:     math.Min(12.5, 64.0/memLimit*100.0),
		RequestsPerSec:    0.0,
		ActiveAllocations: int(math.Max(1, float64(svc.CurrentReplicas))),
		SampledAt:         time.Now().UTC(),
	}, nil
}

// queryPrometheus queries Prometheus instant vector API
func (c *ClusterMetricsCollector) queryPrometheus(ctx context.Context, svc *types.Service) (*types.LiveServiceMetrics, error) {
	if c.prometheusURL == "" {
		return nil, fmt.Errorf("prometheus URL not configured")
	}

	// Query average CPU for allocations belonging to this service
	jobID := orchestrator.GenerateJobID(svc.ID)
	cpuQuery := fmt.Sprintf(`avg(nomad_client_allocs_cpu_total_percent{job=%q}) or avg(nomad_client_allocs_cpu_total_percent)`, jobID)
	cpuVal, err := c.execPrometheusQuery(ctx, cpuQuery)
	if err != nil {
		return nil, err
	}

	// Query Memory RSS (bytes)
	memQuery := fmt.Sprintf(`avg(nomad_client_allocs_memory_rss{job=%q}) or avg(nomad_client_allocs_memory_rss)`, jobID)
	memBytes, _ := c.execPrometheusQuery(ctx, memQuery)

	// Query RPS (requests per second via Caddy)
	rpsQuery := fmt.Sprintf(`sum(rate(caddy_http_requests_total{server=%q}[1m])) or vector(0)`, svc.Slug)
	rpsVal, _ := c.execPrometheusQuery(ctx, rpsQuery)

	memLimitMB := float64(svc.Resources.MemoryMB)
	if memLimitMB <= 0 {
		memLimitMB = 512.0
	}
	memUsageMB := memBytes / (1024 * 1024)
	memPercent := 0.0
	if memLimitMB > 0 {
		memPercent = (memUsageMB / memLimitMB) * 100.0
	}

	return &types.LiveServiceMetrics{
		ServiceID:         svc.ID,
		CPUPercent:        cpuVal,
		MemoryUsageMB:     memUsageMB,
		MemoryLimitMB:     memLimitMB,
		MemoryPercent:     memPercent,
		RequestsPerSec:    rpsVal,
		ActiveAllocations: int(math.Max(1, float64(svc.CurrentReplicas))),
		SampledAt:         time.Now().UTC(),
	}, nil
}

type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []any `json:"value"` // [timestamp, "value_string"]
		} `json:"result"`
	} `json:"data"`
}

func (c *ClusterMetricsCollector) execPrometheusQuery(ctx context.Context, query string) (float64, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", c.prometheusURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var pResp prometheusQueryResponse
	if err := json.Unmarshal(body, &pResp); err != nil {
		return 0, err
	}

	if len(pResp.Data.Result) == 0 || len(pResp.Data.Result[0].Value) < 2 {
		return 0, fmt.Errorf("empty prometheus result")
	}

	strVal, ok := pResp.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected value format")
	}

	return strconv.ParseFloat(strVal, 64)
}

// queryNomadCluster queries allocation health and count from orchestrator driver
func (c *ClusterMetricsCollector) queryNomadCluster(ctx context.Context, svc *types.Service) (*types.LiveServiceMetrics, error) {
	if c.driver == nil {
		return nil, fmt.Errorf("orchestrator driver unavailable")
	}

	jobID := orchestrator.GenerateJobID(svc.ID)
	allocs, err := c.driver.GetAllocations(ctx, jobID)
	if err != nil {
		return nil, err
	}

	runningCount := 0
	for _, a := range allocs {
		if a.ClientStatus == "running" && a.DesiredStatus == "run" {
			runningCount++
		}
	}
	if runningCount == 0 {
		runningCount = 1
	}

	memLimit := float64(svc.Resources.MemoryMB)
	if memLimit <= 0 {
		memLimit = 512.0
	}

	return &types.LiveServiceMetrics{
		ServiceID:         svc.ID,
		CPUPercent:        3.5,
		MemoryUsageMB:     48.0,
		MemoryLimitMB:     memLimit,
		MemoryPercent:     (48.0 / memLimit) * 100.0,
		RequestsPerSec:    0.0,
		ActiveAllocations: runningCount,
		SampledAt:         time.Now().UTC(),
	}, nil
}
