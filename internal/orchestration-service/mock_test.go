package orchestrationservice

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type mockRepository struct {
	mu            sync.RWMutex
	deployments   map[string]types.Deployment
	allocations   map[string][]types.Allocation
	events        map[string][]types.DeploymentEvent
	services      map[string]types.Service
	builds        map[string]types.Build
	envVars       map[string]map[string]string // serviceID -> k/v
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		deployments: make(map[string]types.Deployment),
		allocations: make(map[string][]types.Allocation),
		events:      make(map[string][]types.DeploymentEvent),
		services:    make(map[string]types.Service),
		builds:      make(map[string]types.Build),
		envVars:     make(map[string]map[string]string),
	}
}

func (m *mockRepository) CreateDeployment(ctx context.Context, d *types.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	m.deployments[d.ID] = *d
	return nil
}

func (m *mockRepository) GetDeployment(ctx context.Context, id string) (*types.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.deployments[id]
	if !ok {
		return nil, ErrDeploymentNotFound
	}
	return &d, nil
}

func (m *mockRepository) GetLatestDeploymentForService(ctx context.Context, serviceID string) (*types.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *types.Deployment
	for _, d := range m.deployments {
		if d.ServiceID == serviceID {
			if latest == nil || d.CreatedAt.After(latest.CreatedAt) {
				cp := d
				latest = &cp
			}
		}
	}
	if latest == nil {
		return nil, ErrDeploymentNotFound
	}
	return latest, nil
}

func (m *mockRepository) GetLatestSucceededBuildForService(ctx context.Context, serviceID string) (*types.Build, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *types.Build
	for _, b := range m.builds {
		if b.ServiceID == serviceID && b.Status == types.BuildStatusSucceeded {
			if latest == nil || b.CreatedAt.After(latest.CreatedAt) {
				cp := b
				latest = &cp
			}
		}
	}
	if latest == nil {
		return nil, ErrNoSucceededBuild
	}
	return latest, nil
}

func (m *mockRepository) ListDeploymentsByService(ctx context.Context, serviceID string, limit, offset int) ([]types.Deployment, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []types.Deployment
	for _, d := range m.deployments {
		if d.ServiceID == serviceID {
			filtered = append(filtered, d)
		}
	}
	total := len(filtered)
	if offset >= total {
		return []types.Deployment{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (m *mockRepository) UpdateDeploymentStatus(ctx context.Context, id string, status types.DeploymentStatus, nomadJobID, activeURL, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deployments[id]
	if !ok {
		return ErrDeploymentNotFound
	}
	d.Status = status
	if nomadJobID != "" {
		d.NomadJobID = nomadJobID
	}
	if activeURL != "" {
		d.ActiveURL = activeURL
	}
	if errorMsg != "" {
		d.ErrorMessage = errorMsg
	}
	d.UpdatedAt = time.Now().UTC()
	m.deployments[id] = d
	return nil
}

func (m *mockRepository) UpdateDeploymentJobSpec(ctx context.Context, id string, spec string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deployments[id]
	if !ok {
		return ErrDeploymentNotFound
	}
	d.JobSpec = spec
	d.UpdatedAt = time.Now().UTC()
	m.deployments[id] = d
	return nil
}

func (m *mockRepository) UpdateDeploymentCanaryPromoted(ctx context.Context, id string, promoted bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deployments[id]
	if !ok {
		return ErrDeploymentNotFound
	}
	d.CanaryPromoted = promoted
	d.UpdatedAt = time.Now().UTC()
	m.deployments[id] = d
	return nil
}

func (m *mockRepository) UpsertAllocation(ctx context.Context, a *types.Allocation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.allocations[a.DeploymentID]
	for i, existing := range list {
		if existing.NomadAllocID == a.NomadAllocID {
			list[i] = *a
			m.allocations[a.DeploymentID] = list
			return nil
		}
	}
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	m.allocations[a.DeploymentID] = append(m.allocations[a.DeploymentID], *a)
	return nil
}

func (m *mockRepository) ListAllocations(ctx context.Context, deploymentID string) ([]types.Allocation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.allocations[deploymentID], nil
}

func (m *mockRepository) CreateDeploymentEvent(ctx context.Context, event *types.DeploymentEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	m.events[event.DeploymentID] = append(m.events[event.DeploymentID], *event)
	return nil
}

func (m *mockRepository) ListDeploymentEvents(ctx context.Context, deploymentID string) ([]types.DeploymentEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.events[deploymentID], nil
}

func (m *mockRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.services[serviceID]
	if !ok {
		return nil, ErrServiceNotFound
	}
	return &s, nil
}

func (m *mockRepository) GetBuild(ctx context.Context, buildID string) (*types.Build, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.builds[buildID]
	if !ok {
		return nil, ErrBuildNotFound
	}
	return &b, nil
}

func (m *mockRepository) GetEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]string)
	for k, v := range m.envVars[serviceID] {
		res[k] = v
	}
	return res, nil
}

func (m *mockRepository) GetEnvironmentVariablesDetailed(ctx context.Context, serviceID string, projectID *string) (map[string]string, []string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]string)
	for k, v := range m.envVars[serviceID] {
		res[k] = v
	}
	return res, nil, nil
}

func (m *mockRepository) CreateDeploymentEnvSnapshot(ctx context.Context, snapshot *types.DeploymentEnvSnapshot) error {
	return nil
}

func (m *mockRepository) GetDeploymentEnvSnapshot(ctx context.Context, deploymentID string) (*types.DeploymentEnvSnapshot, error) {
	return nil, ErrDeploymentNotFound
}


func (m *mockRepository) UpdateServiceActiveDeployment(ctx context.Context, serviceID, deploymentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[serviceID]
	if !ok {
		return ErrServiceNotFound
	}
	s.ActiveDeploymentID = &deploymentID
	m.services[serviceID] = s
	return nil
}

func (m *mockRepository) UpdateServiceReplicas(ctx context.Context, serviceID string, replicas int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[serviceID]
	if !ok {
		return ErrServiceNotFound
	}
	s.MinReplicas = replicas
	s.MaxReplicas = replicas
	m.services[serviceID] = s
	return nil
}

func (m *mockRepository) MarkPreviousDeploymentsReplaced(ctx context.Context, serviceID, currentDeploymentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.deployments {
		if d.ServiceID == serviceID && id != currentDeploymentID && d.Status == types.DeploymentStatusActive {
			d.Status = types.DeploymentStatusReplaced
			m.deployments[id] = d
		}
	}
	return nil
}

func (m *mockRepository) CreateBuild(ctx context.Context, b *types.Build) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	m.builds[b.ID] = *b
	return nil
}
