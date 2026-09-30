package logservice

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type MockRepository struct {
	mu          sync.RWMutex
	services    map[string]*types.Service
	logs        []types.AppLogEntry
	userAccess  map[string]map[string]bool // userID -> orgID -> bool
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		services:   make(map[string]*types.Service),
		logs:       make([]types.AppLogEntry, 0),
		userAccess: make(map[string]map[string]bool),
	}
}

func (m *MockRepository) AddService(s *types.Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services[s.ID] = s
}

func (m *MockRepository) SetUserAccess(userID, orgID string, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.userAccess[userID]; !ok {
		m.userAccess[userID] = make(map[string]bool)
	}
	m.userAccess[userID][orgID] = allowed
}

func (m *MockRepository) InsertLog(ctx context.Context, entry types.AppLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = "log_" + uuid.New().String()[:12]
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if entry.Stream == "" {
		entry.Stream = "stdout"
	}

	m.logs = append(m.logs, entry)
	return nil
}

func (m *MockRepository) InsertLogsBatch(ctx context.Context, entries []types.AppLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, entry := range entries {
		if entry.ID == "" {
			entry.ID = "log_" + uuid.New().String()[:12]
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = now
		}
		if entry.Stream == "" {
			entry.Stream = "stdout"
		}
		m.logs = append(m.logs, entry)
	}
	return nil
}

func (m *MockRepository) GetLogs(ctx context.Context, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []types.AppLogEntry
	for _, l := range m.logs {
		if filter.ServiceID != "" && l.ServiceID != filter.ServiceID {
			continue
		}
		if filter.DeploymentID != "" && (l.DeploymentID == nil || *l.DeploymentID != filter.DeploymentID) {
			continue
		}
		if filter.AllocationID != "" && (l.AllocationID == nil || *l.AllocationID != filter.AllocationID) {
			continue
		}
		if filter.Stream != "" && filter.Stream != "all" && l.Stream != filter.Stream {
			continue
		}
		if filter.Search != "" && !strings.Contains(strings.ToLower(l.Message), strings.ToLower(filter.Search)) {
			continue
		}
		if filter.Since != nil && l.Timestamp.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && l.Timestamp.After(*filter.Until) {
			continue
		}
		matched = append(matched, l)
	}

	total := len(matched)
	offset := filter.Offset
	if offset > total {
		return []types.AppLogEntry{}, total, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return matched[offset:end], total, nil
}

func (m *MockRepository) GetDeploymentLogs(ctx context.Context, deploymentID string, filter types.AppLogsQueryFilter) ([]types.AppLogEntry, int, error) {
	filter.DeploymentID = deploymentID
	return m.GetLogs(ctx, filter)
}

func (m *MockRepository) PurgeLogs(ctx context.Context, serviceID string, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var remaining []types.AppLogEntry
	var deleted int64

	for _, l := range m.logs {
		if l.ServiceID == serviceID && l.Timestamp.Before(before) {
			deleted++
		} else {
			remaining = append(remaining, l)
		}
	}

	m.logs = remaining
	return deleted, nil
}

func (m *MockRepository) GetService(ctx context.Context, serviceID string) (*types.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	svc, ok := m.services[serviceID]
	if !ok {
		return nil, ErrServiceNotFound
	}
	return svc, nil
}

func (m *MockRepository) UserHasOrgAccess(ctx context.Context, userID, orgID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if orgs, ok := m.userAccess[userID]; ok {
		return orgs[orgID], nil
	}
	return false, nil
}
