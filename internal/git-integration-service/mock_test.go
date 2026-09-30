package gitintegrationservice

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type MockRepository struct {
	mu            sync.Mutex
	services      map[string]types.Service
	builds        []types.Build
	providers     map[string]map[types.GitProviderType]providerEntry
	webhookEvents []types.GitWebhookEventRecord
}

type providerEntry struct {
	conn  types.GitProviderConnection
	token string
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		services:  make(map[string]types.Service),
		providers: make(map[string]map[types.GitProviderType]providerEntry),
	}
}

func (m *MockRepository) FindMatchingServices(ctx context.Context, repoCanonical, branch string) ([]types.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []types.Service
	for _, svc := range m.services {
		if GitURLsMatch(svc.RepoURL, repoCanonical) {
			if svc.Branch == "" || svc.Branch == branch {
				matched = append(matched, svc)
			}
		}
	}
	return matched, nil
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

func (m *MockRepository) UpdateServiceWebhookSecret(ctx context.Context, serviceID, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, ok := m.services[serviceID]
	if !ok {
		return ErrNotFound
	}
	svc.WebhookSecret = secret
	m.services[serviceID] = svc
	return nil
}

func (m *MockRepository) CreateBuild(ctx context.Context, build *types.Build) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if build.ID == "" {
		build.ID = uuid.New().String()
	}
	if build.CreatedAt.IsZero() {
		build.CreatedAt = time.Now().UTC()
	}
	m.builds = append(m.builds, *build)
	return nil
}

func (m *MockRepository) SaveGitProvider(ctx context.Context, conn *types.GitProviderConnection, accessToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.providers[conn.UserID] == nil {
		m.providers[conn.UserID] = make(map[types.GitProviderType]providerEntry)
	}
	if conn.ID == "" {
		conn.ID = uuid.New().String()
	}
	conn.CreatedAt = time.Now().UTC()
	conn.UpdatedAt = time.Now().UTC()

	m.providers[conn.UserID][conn.Provider] = providerEntry{
		conn:  *conn,
		token: accessToken,
	}
	return nil
}

func (m *MockRepository) GetGitProvider(ctx context.Context, userID string, provider types.GitProviderType) (*types.GitProviderConnection, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userMap, ok := m.providers[userID]
	if !ok {
		return nil, "", ErrNotFound
	}
	entry, ok := userMap[provider]
	if !ok {
		return nil, "", ErrNotFound
	}
	return &entry.conn, entry.token, nil
}

func (m *MockRepository) ListGitProviders(ctx context.Context, userID string) ([]types.GitProviderConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userMap := m.providers[userID]
	var list []types.GitProviderConnection
	for _, entry := range userMap {
		list = append(list, entry.conn)
	}
	return list, nil
}

func (m *MockRepository) DeleteGitProvider(ctx context.Context, userID string, provider types.GitProviderType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if userMap, ok := m.providers[userID]; ok {
		delete(userMap, provider)
	}
	return nil
}

func (m *MockRepository) RecordWebhookEvent(ctx context.Context, event *types.GitWebhookEventRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	m.webhookEvents = append(m.webhookEvents, *event)
	return nil
}

func (m *MockRepository) ListWebhookEvents(ctx context.Context, limit int) ([]types.GitWebhookEventRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.webhookEvents) <= limit {
		return m.webhookEvents, nil
	}
	return m.webhookEvents[:limit], nil
}

// MockGitClient simulates remote calls to GitHub / GitLab API
type MockGitClient struct {
	UserInfo       *types.GitProviderConnection
	Repositories   []types.GitRepository
	Branches       []types.GitBranch
	CommitInfo     *types.GitCommitInfo
	CreatedHookID  string
	ShouldFailUser bool
	ShouldFailHook bool
}

func (m *MockGitClient) GetUserInfo(ctx context.Context, token string) (*types.GitProviderConnection, error) {
	if m.ShouldFailUser {
		return nil, errors.New("unauthorized token")
	}
	if m.UserInfo != nil {
		return m.UserInfo, nil
	}
	return &types.GitProviderConnection{
		Username:    "octocat",
		DisplayName: "The Octocat",
		AvatarURL:   "https://avatars.githubusercontent.com/u/583231?v=4",
	}, nil
}

func (m *MockGitClient) ListRepositories(ctx context.Context, token string, page, perPage int) ([]types.GitRepository, error) {
	return m.Repositories, nil
}

func (m *MockGitClient) ListBranches(ctx context.Context, token string, owner, repo string) ([]types.GitBranch, error) {
	return m.Branches, nil
}

func (m *MockGitClient) GetCommit(ctx context.Context, token string, owner, repo, ref string) (*types.GitCommitInfo, error) {
	return m.CommitInfo, nil
}

func (m *MockGitClient) CreateWebhook(ctx context.Context, token string, owner, repo string, webhookURL, secret string) (string, error) {
	if m.ShouldFailHook {
		return "", errors.New("insufficient permissions to create webhook")
	}
	if m.CreatedHookID != "" {
		return m.CreatedHookID, nil
	}
	return "hook_12345", nil
}
