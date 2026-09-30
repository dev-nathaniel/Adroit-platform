package dnsservice

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type MockRepository struct {
	mu            sync.RWMutex
	domains       map[string]*types.CustomDomain
	ingressRoutes map[string]*types.IngressRoute
	services      map[string]*types.Service
	deployments   map[string]*types.Deployment
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		domains:       make(map[string]*types.CustomDomain),
		ingressRoutes: make(map[string]*types.IngressRoute),
		services:      make(map[string]*types.Service),
		deployments:   make(map[string]*types.Deployment),
	}
}

func (m *MockRepository) CreateDomain(_ context.Context, d *types.CustomDomain) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanDomain := strings.ToLower(strings.TrimSpace(d.Domain))
	for _, existing := range m.domains {
		if existing.Domain == cleanDomain {
			return ErrDomainAlreadyExists
		}
	}

	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now
	d.Domain = cleanDomain

	cp := *d
	m.domains[d.ID] = &cp
	return nil
}

func (m *MockRepository) GetDomainByID(_ context.Context, id string) (*types.CustomDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.domains[id]
	if !ok {
		return nil, ErrDomainNotFound
	}
	cp := *d
	return &cp, nil
}

func (m *MockRepository) GetDomainByName(_ context.Context, domain string) (*types.CustomDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(domain))
	for _, d := range m.domains {
		if d.Domain == clean {
			cp := *d
			return &cp, nil
		}
	}
	return nil, ErrDomainNotFound
}

func (m *MockRepository) ListDomainsByService(_ context.Context, serviceID string, limit, offset int) ([]types.CustomDomain, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matching []types.CustomDomain
	for _, d := range m.domains {
		if d.ServiceID == serviceID {
			matching = append(matching, *d)
		}
	}

	total := len(matching)
	if offset >= total {
		return []types.CustomDomain{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return matching[offset:end], total, nil
}

func (m *MockRepository) UpdateDomainStatus(_ context.Context, id string, status types.DomainStatus, sslStatus types.SSLStatus, sslError string, verifiedAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, ok := m.domains[id]
	if !ok {
		return ErrDomainNotFound
	}
	d.Status = status
	d.SSLStatus = sslStatus
	d.SSLError = sslError
	d.VerifiedAt = verifiedAt
	d.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MockRepository) DeleteDomain(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.domains[id]; !ok {
		return ErrDomainNotFound
	}
	delete(m.domains, id)
	return nil
}

func (m *MockRepository) SetPrimaryDomain(_ context.Context, serviceID, domainID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	found := false
	for _, d := range m.domains {
		if d.ServiceID == serviceID {
			if d.ID == domainID {
				d.IsPrimary = true
				found = true
			} else {
				d.IsPrimary = false
			}
			d.UpdatedAt = time.Now().UTC()
		}
	}
	if !found {
		return ErrDomainNotFound
	}
	return nil
}

func (m *MockRepository) UpsertIngressRoute(_ context.Context, route *types.IngressRoute) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if route.ID == "" {
		route.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	route.LastSyncedAt = now
	route.HostPattern = strings.ToLower(strings.TrimSpace(route.HostPattern))

	cp := *route
	m.ingressRoutes[route.HostPattern] = &cp
	return nil
}

func (m *MockRepository) GetIngressRouteByHost(_ context.Context, hostPattern string) (*types.IngressRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(hostPattern))
	r, ok := m.ingressRoutes[clean]
	if !ok {
		return nil, ErrRouteNotFound
	}
	cp := *r
	return &cp, nil
}

func (m *MockRepository) ListIngressRoutesByService(_ context.Context, serviceID string) ([]types.IngressRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matching []types.IngressRoute
	for _, r := range m.ingressRoutes {
		if r.ServiceID == serviceID {
			matching = append(matching, *r)
		}
	}
	return matching, nil
}

func (m *MockRepository) ListAllIngressRoutes(_ context.Context, limit, offset int) ([]types.IngressRoute, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []types.IngressRoute
	for _, r := range m.ingressRoutes {
		list = append(list, *r)
	}

	total := len(list)
	if offset >= total {
		return []types.IngressRoute{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return list[offset:end], total, nil
}

func (m *MockRepository) DeleteIngressRoute(_ context.Context, hostPattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := strings.ToLower(strings.TrimSpace(hostPattern))
	if _, ok := m.ingressRoutes[clean]; !ok {
		return ErrRouteNotFound
	}
	delete(m.ingressRoutes, clean)
	return nil
}

func (m *MockRepository) GetService(_ context.Context, serviceID string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.services[serviceID]
	if !ok {
		return nil, ErrServiceNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *MockRepository) GetActiveDeploymentForService(_ context.Context, serviceID string) (*types.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, d := range m.deployments {
		if d.ServiceID == serviceID && d.Status == types.DeploymentStatusActive {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}
