package buildservice

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type MockRepository struct {
	mu          sync.Mutex
	services    map[string]types.Service
	orgMembers  map[string]map[string]string // orgID -> userID -> role
	orgSlugs    map[string]string            // orgID -> slug
	builds      map[string]types.Build
	buildLogs   map[string][]types.BuildLogEntry // buildID -> entries
	deployments map[string]types.Deployment
	envVars     map[string]map[string]string // serviceID -> k -> v
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		services:    make(map[string]types.Service),
		orgMembers:  make(map[string]map[string]string),
		orgSlugs:    make(map[string]string),
		builds:      make(map[string]types.Build),
		buildLogs:   make(map[string][]types.BuildLogEntry),
		deployments: make(map[string]types.Deployment),
		envVars:     make(map[string]map[string]string),
	}
}

func (m *MockRepository) CheckOrgMembership(ctx context.Context, orgID, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	users, ok := m.orgMembers[orgID]
	if !ok {
		return "", ErrForbidden
	}
	role, ok := users[userID]
	if !ok {
		return "", ErrForbidden
	}
	return role, nil
}

func (m *MockRepository) GetServiceByID(ctx context.Context, serviceID string) (*types.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, ok := m.services[serviceID]
	if !ok {
		return nil, ErrNotFound
	}
	return &svc, nil
}

func (m *MockRepository) GetOrganizationSlug(ctx context.Context, orgID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	slug, ok := m.orgSlugs[orgID]
	if !ok {
		return "default", nil
	}
	return slug, nil
}

func (m *MockRepository) CreateBuild(ctx context.Context, b *types.Build) error {
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

func (m *MockRepository) GetBuildByID(ctx context.Context, buildID string) (*types.Build, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.builds[buildID]
	if !ok {
		return nil, ErrNotFound
	}
	return &b, nil
}

func (m *MockRepository) ListBuildsByService(ctx context.Context, serviceID string, limit, offset int, status string) ([]types.Build, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []types.Build
	for _, b := range m.builds {
		if b.ServiceID == serviceID {
			if status == "" || strings.EqualFold(string(b.Status), status) {
				matched = append(matched, b)
			}
		}
	}

	total := len(matched)
	if offset >= total {
		return []types.Build{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return matched[offset:end], total, nil
}

func (m *MockRepository) UpdateBuildStatus(ctx context.Context, buildID string, status types.BuildStatus, imageTag, imageDigest, errorMessage string, durationMs int64, finishedAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.builds[buildID]
	if !ok {
		return ErrNotFound
	}

	b.Status = status
	if imageTag != "" {
		b.ImageTag = imageTag
	}
	if imageDigest != "" {
		b.ImageDigest = imageDigest
	}
	if errorMessage != "" {
		b.ErrorMessage = errorMessage
	}
	if durationMs > 0 {
		b.DurationMs = durationMs
	}
	if finishedAt != nil {
		b.FinishedAt = finishedAt
	}

	m.builds[buildID] = b
	return nil
}

func (m *MockRepository) InsertBuildLog(ctx context.Context, entry *types.BuildLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	m.buildLogs[entry.BuildID] = append(m.buildLogs[entry.BuildID], *entry)
	return nil
}

func (m *MockRepository) InsertBuildLogsBatch(ctx context.Context, entries []types.BuildLogEntry) error {
	for _, e := range entries {
		if err := m.InsertBuildLog(ctx, &e); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockRepository) GetBuildLogs(ctx context.Context, buildID string, limit, offset int) ([]types.BuildLogEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	logs, ok := m.buildLogs[buildID]
	if !ok {
		return []types.BuildLogEntry{}, nil
	}

	total := len(logs)
	if offset >= total {
		return []types.BuildLogEntry{}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return logs[offset:end], nil
}

func (m *MockRepository) CreateDeployment(ctx context.Context, d *types.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	m.deployments[d.ID] = *d
	return nil
}

func (m *MockRepository) GetServiceEnvVars(ctx context.Context, serviceID string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	vars, ok := m.envVars[serviceID]
	if !ok {
		return make(map[string]string), nil
	}
	res := make(map[string]string)
	for k, v := range vars {
		res[k] = v
	}
	return res, nil
}

func (m *MockRepository) GetBuildEnvironmentVariables(ctx context.Context, serviceID string, projectID *string) (map[string]string, error) {
	return m.GetServiceEnvVars(ctx, serviceID)
}

