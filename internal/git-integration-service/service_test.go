package gitintegrationservice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func createTestHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestProcessGitHubWebhook_Success(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()
	bus := events.NewInMemBus()
	defer bus.Close()

	// Register a service
	svcID := "svc-test-1"
	repo.services[svcID] = types.Service{
		ID:             svcID,
		OrganizationID: "org-1",
		Name:           "Core API",
		Slug:           "core-api",
		RepoURL:        "https://github.com/adroit-platform/core-api",
		Branch:         "main",
		WebhookSecret:  "test-secret-123",
	}

	svc := NewService(repo, ServiceConfig{
		PlatformURL: "http://localhost:8080",
	}, WithEventBus(bus))

	// Listen for NATS events
	buildRequestedCh := make(chan events.Event[events.BuildRequestedPayload], 1)
	err := bus.Subscribe(events.SubjectBuildRequested, func(ctx context.Context, subj string, rawData []byte) error {
		var ev events.Event[events.BuildRequestedPayload]
		if err := json.Unmarshal(rawData, &ev); err == nil {
			buildRequestedCh <- ev
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to subscribe to events: %v", err)
	}

	// Construct push payload
	pushPayload := GitHubPushPayload{
		Ref:   "refs/heads/main",
		After: "7c3a4f6e1b2d8c90",
		Repository: GitHubRepoPayload{
			CloneURL: "https://github.com/adroit-platform/core-api.git",
			FullName: "adroit-platform/core-api",
		},
		Pusher: GitHubPusherPayload{
			Name:  "dev-user",
			Email: "dev@adroit.run",
		},
		HeadCommit: &GitHubCommitPayload{
			ID:      "7c3a4f6e1b2d8c90",
			Message: "feat: add user authentication",
			Author: GitHubAuthorPayload{
				Name:  "Alice Dev",
				Email: "alice@adroit.run",
			},
		},
	}
	body, _ := json.Marshal(pushPayload)
	sig := createTestHMAC("test-secret-123", body)

	res, err := svc.ProcessGitHubWebhook(ctx, "del-123", "push", sig, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "processed" {
		t.Errorf("expected status processed, got %s", res.Status)
	}
	if len(res.MatchedServices) != 1 || res.MatchedServices[0] != svcID {
		t.Errorf("expected matched service %s, got %v", svcID, res.MatchedServices)
	}
	if len(res.BuildsTriggered) != 1 {
		t.Fatalf("expected 1 triggered build, got %d", len(res.BuildsTriggered))
	}

	// Verify build in repository
	if len(repo.builds) != 1 {
		t.Fatalf("expected 1 build in repo, got %d", len(repo.builds))
	}
	bld := repo.builds[0]
	if bld.CommitHash != "7c3a4f6e1b2d8c90" {
		t.Errorf("expected commit hash 7c3a4f6e1b2d8c90, got %s", bld.CommitHash)
	}
	if bld.TriggeredBy != "webhook:github" {
		t.Errorf("expected triggered_by webhook:github, got %s", bld.TriggeredBy)
	}

	// Verify event published
	select {
	case evt := <-buildRequestedCh:
		if evt.Data.ServiceID != svcID {
			t.Errorf("expected event service id %s, got %s", svcID, evt.Data.ServiceID)
		}
		if evt.Data.CommitHash != "7c3a4f6e1b2d8c90" {
			t.Errorf("expected event commit hash 7c3a4f6e1b2d8c90, got %s", evt.Data.CommitHash)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SubjectBuildRequested event")
	}
}

func TestProcessGitHubWebhook_InvalidSignature(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()

	repo.services["svc-1"] = types.Service{
		ID:            "svc-1",
		RepoURL:       "https://github.com/adroit/app",
		Branch:        "main",
		WebhookSecret: "correct-secret",
	}

	svc := NewService(repo, ServiceConfig{})

	payload := GitHubPushPayload{
		Ref:   "refs/heads/main",
		After: "123456",
		Repository: GitHubRepoPayload{
			CloneURL: "https://github.com/adroit/app.git",
		},
	}
	body, _ := json.Marshal(payload)

	_, err := svc.ProcessGitHubWebhook(ctx, "del-1", "push", "sha256=invalidmac", body)
	if err == nil {
		t.Fatal("expected unauthorized error for invalid signature, got nil")
	}
}

func TestProcessGitHubWebhook_Ping(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()
	svc := NewService(repo, ServiceConfig{})

	res, err := svc.ProcessGitHubWebhook(ctx, "del-ping", "ping", "", []byte(`{"zen":"Design for failure."}`))
	if err != nil {
		t.Fatalf("unexpected error on ping: %v", err)
	}
	if res.Status != "ok" || res.Event != "ping" {
		t.Errorf("unexpected ping response: %+v", res)
	}
}

func TestProcessGitLabWebhook_Success(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()
	bus := events.NewInMemBus()
	defer bus.Close()

	svcID := "svc-gl-1"
	repo.services[svcID] = types.Service{
		ID:             svcID,
		OrganizationID: "org-1",
		Name:           "GitLab App",
		RepoURL:        "https://gitlab.com/acme/backend/billing-api",
		Branch:         "main",
		WebhookSecret:  "gl-token-secret-999",
	}

	svc := NewService(repo, ServiceConfig{}, WithEventBus(bus))

	payload := GitLabPushPayload{
		ObjectKind:  "push",
		CheckoutSHA: "987654321fedcba",
		Ref:         "refs/heads/main",
		UserName:    "GitLab Contributor",
		UserEmail:   "gl@example.com",
		Project: GitLabProjectPayload{
			GitHTTPURL: "https://gitlab.com/acme/backend/billing-api.git",
		},
		Commits: []GitLabCommitPayload{
			{
				ID:      "987654321fedcba",
				Message: "refactor: optimize database queries",
				Author: GitLabAuthorPayload{
					Name:  "GitLab Contributor",
					Email: "gl@example.com",
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	res, err := svc.ProcessGitLabWebhook(ctx, "del-gl-1", "Push Hook", "gl-token-secret-999", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "processed" {
		t.Errorf("expected status processed, got %s", res.Status)
	}
	if len(res.BuildsTriggered) != 1 {
		t.Fatalf("expected 1 triggered build, got %d", len(res.BuildsTriggered))
	}

	bld := repo.builds[0]
	if bld.CommitHash != "987654321fedcba" {
		t.Errorf("expected commit 987654321fedcba, got %s", bld.CommitHash)
	}
	if bld.TriggeredBy != "webhook:gitlab" {
		t.Errorf("expected triggered_by webhook:gitlab, got %s", bld.TriggeredBy)
	}
}

func TestProcessGitLabWebhook_InvalidToken(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()

	repo.services["svc-gl"] = types.Service{
		ID:            "svc-gl",
		RepoURL:       "https://gitlab.com/org/proj",
		Branch:        "main",
		WebhookSecret: "super-secure-token",
	}

	svc := NewService(repo, ServiceConfig{})

	payload := GitLabPushPayload{
		ObjectKind: "push",
		Ref:        "refs/heads/main",
		Project: GitLabProjectPayload{
			GitHTTPURL: "https://gitlab.com/org/proj.git",
		},
	}
	body, _ := json.Marshal(payload)

	_, err := svc.ProcessGitLabWebhook(ctx, "del-gl", "Push Hook", "wrong-token", body)
	if err == nil {
		t.Fatal("expected unauthorized error for invalid gitlab token, got nil")
	}
}

func TestConnectAndDiscoverRepositories(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()

	mockGH := &MockGitClient{
		UserInfo: &types.GitProviderConnection{
			Username:    "octocat",
			DisplayName: "Mona Lisa Octocat",
			AvatarURL:   "https://avatars.githubusercontent.com/u/583231?v=4",
		},
		Repositories: []types.GitRepository{
			{
				ID:            "101",
				Name:          "web-platform",
				FullName:      "octocat/web-platform",
				Owner:         "octocat",
				CloneURL:      "https://github.com/octocat/web-platform.git",
				DefaultBranch: "main",
			},
		},
		Branches: []types.GitBranch{
			{Name: "main", CommitSHA: "c111", IsDefault: true},
			{Name: "dev", CommitSHA: "c222", IsDefault: false},
		},
	}

	svc := NewService(repo, ServiceConfig{}, WithGitHubClient(mockGH))

	userID := "user-42"

	// 1. Connect Provider
	conn, err := svc.ConnectProvider(ctx, userID, ConnectProviderRequest{
		Provider:    types.GitProviderGitHub,
		AccessToken: "ghp_mock_token_12345",
	})
	if err != nil {
		t.Fatalf("failed to connect provider: %v", err)
	}
	if conn.Username != "octocat" {
		t.Errorf("expected username octocat, got %s", conn.Username)
	}

	// 2. List Providers
	providers, err := svc.ListProviders(ctx, userID)
	if err != nil || len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d, err: %v", len(providers), err)
	}

	// 3. List Repositories
	repos, err := svc.ListRepositories(ctx, userID, types.GitProviderGitHub, 1, 30)
	if err != nil || len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d, err: %v", len(repos), err)
	}
	if repos[0].Name != "web-platform" {
		t.Errorf("expected web-platform repo, got %s", repos[0].Name)
	}

	// 4. List Branches
	branches, err := svc.ListBranches(ctx, userID, types.GitProviderGitHub, "octocat", "web-platform")
	if err != nil || len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d, err: %v", len(branches), err)
	}
}

func TestSetupServiceWebhook(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository()

	mockGH := &MockGitClient{
		CreatedHookID: "hook-9988",
	}

	svc := NewService(repo, ServiceConfig{
		PlatformURL: "https://api.adroit.run",
	}, WithGitHubClient(mockGH))

	userID := "user-100"
	_ = repo.SaveGitProvider(ctx, &types.GitProviderConnection{
		UserID:   userID,
		Provider: types.GitProviderGitHub,
		Username: "octocat",
	}, "ghp_valid_token")

	repo.services["svc-auto"] = types.Service{
		ID:      "svc-auto",
		RepoURL: "https://github.com/octocat/my-app.git",
		Branch:  "main",
	}

	setupRes, err := svc.SetupServiceWebhook(ctx, userID, "svc-auto")
	if err != nil {
		t.Fatalf("failed to setup service webhook: %v", err)
	}

	if !setupRes.AutoConfigured {
		t.Errorf("expected webhook to be auto-configured")
	}
	if setupRes.WebhookID != "hook-9988" {
		t.Errorf("expected hook id hook-9988, got %s", setupRes.WebhookID)
	}
	if setupRes.WebhookSecret == "" {
		t.Errorf("expected non-empty webhook secret")
	}
}
