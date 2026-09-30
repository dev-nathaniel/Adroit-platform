package projectservice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// MockRepository provides an in-memory thread-safe implementation of Repository for unit tests
type MockRepository struct {
	mu           sync.RWMutex
	members      map[string]map[string]string // orgID -> userID -> role
	projects     map[string]*types.Project    // id -> project
	services     map[string]*types.Service    // id -> service
	serviceEnvs  map[string]map[string]types.EnvironmentVariable // serviceID -> key -> env
	projectEnvs  map[string]map[string]types.EnvironmentVariable // projectID -> key -> env
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		members:     make(map[string]map[string]string),
		projects:    make(map[string]*types.Project),
		services:    make(map[string]*types.Service),
		serviceEnvs: make(map[string]map[string]types.EnvironmentVariable),
		projectEnvs: make(map[string]map[string]types.EnvironmentVariable),
	}
}

func (m *MockRepository) AddMember(orgID, userID, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.members[orgID] == nil {
		m.members[orgID] = make(map[string]string)
	}
	m.members[orgID][userID] = role
}

func (m *MockRepository) CheckOrgMembership(ctx context.Context, orgID, userID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if orgUsers, ok := m.members[orgID]; ok {
		if role, ok := orgUsers[userID]; ok {
			return role, nil
		}
	}
	return "", ErrForbidden
}

func (m *MockRepository) CreateProject(ctx context.Context, p *types.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.projects {
		if existing.OrganizationID == p.OrganizationID && existing.Slug == p.Slug {
			return ErrConflict
		}
	}
	copyP := *p
	m.projects[p.ID] = &copyP
	return nil
}

func (m *MockRepository) GetProjectByID(ctx context.Context, id string) (*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.projects[id]; ok {
		copyP := *p
		return &copyP, nil
	}
	return nil, ErrNotFound
}

func (m *MockRepository) GetProjectBySlug(ctx context.Context, orgID, slug string) (*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.projects {
		if p.OrganizationID == orgID && p.Slug == slug {
			copyP := *p
			return &copyP, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MockRepository) ListProjectsByOrg(ctx context.Context, orgID string) ([]types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.Project
	for _, p := range m.projects {
		if p.OrganizationID == orgID {
			list = append(list, *p)
		}
	}
	return list, nil
}

func (m *MockRepository) ListProjectsByUser(ctx context.Context, userID string) ([]types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.Project
	for _, p := range m.projects {
		if orgUsers, ok := m.members[p.OrganizationID]; ok {
			if _, ok := orgUsers[userID]; ok {
				list = append(list, *p)
			}
		}
	}
	return list, nil
}

func (m *MockRepository) UpdateProject(ctx context.Context, p *types.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[p.ID]; !ok {
		return ErrNotFound
	}
	copyP := *p
	m.projects[p.ID] = &copyP
	return nil
}

func (m *MockRepository) DeleteProject(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[id]; !ok {
		return ErrNotFound
	}
	delete(m.projects, id)
	// Detach services in that project to become standalone
	for _, s := range m.services {
		if s.ProjectID != nil && *s.ProjectID == id {
			s.ProjectID = nil
		}
	}
	delete(m.projectEnvs, id)
	return nil
}

func (m *MockRepository) CountServicesInProject(ctx context.Context, projectID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.services {
		if s.ProjectID != nil && *s.ProjectID == projectID {
			count++
		}
	}
	return count, nil
}

func (m *MockRepository) CreateService(ctx context.Context, s *types.Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.services {
		if existing.OrganizationID == s.OrganizationID && existing.Slug == s.Slug {
			return ErrConflict
		}
	}
	copyS := *s
	m.services[s.ID] = &copyS
	return nil
}

func (m *MockRepository) GetServiceByID(ctx context.Context, id string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.services[id]; ok {
		copyS := *s
		return &copyS, nil
	}
	return nil, ErrNotFound
}

func (m *MockRepository) GetServiceBySlug(ctx context.Context, orgID, slug string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.services {
		if s.OrganizationID == orgID && s.Slug == slug {
			copyS := *s
			return &copyS, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MockRepository) ListServices(ctx context.Context, orgID string, projectID *string, standaloneOnly bool) ([]types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.Service
	for _, s := range m.services {
		if s.OrganizationID != orgID {
			continue
		}
		if standaloneOnly {
			if s.ProjectID == nil {
				list = append(list, *s)
			}
			continue
		}
		if projectID != nil && *projectID != "" {
			if s.ProjectID != nil && *s.ProjectID == *projectID {
				list = append(list, *s)
			}
			continue
		}
		list = append(list, *s)
	}
	return list, nil
}

func (m *MockRepository) UpdateService(ctx context.Context, s *types.Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[s.ID]; !ok {
		return ErrNotFound
	}
	copyS := *s
	m.services[s.ID] = &copyS
	return nil
}

func (m *MockRepository) MoveService(ctx context.Context, serviceID string, newProjectID *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[serviceID]
	if !ok {
		return ErrNotFound
	}
	if newProjectID != nil {
		pCopy := *newProjectID
		s.ProjectID = &pCopy
	} else {
		s.ProjectID = nil
	}
	s.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MockRepository) DeleteService(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[id]; !ok {
		return ErrNotFound
	}
	delete(m.services, id)
	delete(m.serviceEnvs, id)
	return nil
}

func (m *MockRepository) SetServiceEnvVars(ctx context.Context, orgID, serviceID string, vars []EnvVarItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.serviceEnvs[serviceID] == nil {
		m.serviceEnvs[serviceID] = make(map[string]types.EnvironmentVariable)
	}
	now := time.Now().UTC()
	for _, v := range vars {
		m.serviceEnvs[serviceID][v.Key] = types.EnvironmentVariable{
			ID:             fmt.Sprintf("env_%s_%s", serviceID, v.Key),
			OrganizationID: orgID,
			ServiceID:      &serviceID,
			Key:            v.Key,
			Value:          v.Value,
			IsSecret:       v.IsSecret,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	return nil
}

func (m *MockRepository) GetServiceEnvVars(ctx context.Context, serviceID string) ([]types.EnvironmentVariable, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.EnvironmentVariable
	if envs, ok := m.serviceEnvs[serviceID]; ok {
		for _, ev := range envs {
			list = append(list, ev)
		}
	}
	return list, nil
}

func (m *MockRepository) DeleteServiceEnvVar(ctx context.Context, serviceID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if envs, ok := m.serviceEnvs[serviceID]; ok {
		if _, ok := envs[key]; ok {
			delete(envs, key)
			return nil
		}
	}
	return ErrNotFound
}

func (m *MockRepository) SetProjectEnvVars(ctx context.Context, orgID, projectID string, vars []EnvVarItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.projectEnvs[projectID] == nil {
		m.projectEnvs[projectID] = make(map[string]types.EnvironmentVariable)
	}
	now := time.Now().UTC()
	for _, v := range vars {
		m.projectEnvs[projectID][v.Key] = types.EnvironmentVariable{
			ID:             fmt.Sprintf("env_%s_%s", projectID, v.Key),
			OrganizationID: orgID,
			ProjectID:      &projectID,
			Key:            v.Key,
			Value:          v.Value,
			IsSecret:       v.IsSecret,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	return nil
}

func (m *MockRepository) GetProjectEnvVars(ctx context.Context, projectID string) ([]types.EnvironmentVariable, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []types.EnvironmentVariable
	if envs, ok := m.projectEnvs[projectID]; ok {
		for _, ev := range envs {
			list = append(list, ev)
		}
	}
	return list, nil
}

func (m *MockRepository) DeleteProjectEnvVar(ctx context.Context, projectID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if envs, ok := m.projectEnvs[projectID]; ok {
		if _, ok := envs[key]; ok {
			delete(envs, key)
			return nil
		}
	}
	return ErrNotFound
}
