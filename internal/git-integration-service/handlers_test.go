package gitintegrationservice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

const testJWTSecret = "test-jwt-signing-secret-for-handlers-32-bytes"

func setupTestRouter(svc *Service) *chi.Mux {
	r := chi.NewRouter()
	h := NewHandler(svc)
	h.RegisterRoutes(r, testJWTSecret)
	return r
}

func createAuthToken(t *testing.T, userID, email string) string {
	token, err := identity.GenerateJWT(userID, email, testJWTSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return token
}

func TestHandler_GitHubWebhook(t *testing.T) {
	repo := NewMockRepository()
	repo.services["svc-1"] = types.Service{
		ID:            "svc-1",
		RepoURL:       "https://github.com/org/repo.git",
		Branch:        "main",
		WebhookSecret: "gh-secret",
	}

	svc := NewService(repo, ServiceConfig{})
	router := setupTestRouter(svc)

	payload := GitHubPushPayload{
		Ref:   "refs/heads/main",
		After: "commit123",
		Repository: GitHubRepoPayload{
			CloneURL: "https://github.com/org/repo.git",
		},
	}
	body, _ := json.Marshal(payload)
	sig := createTestHMAC("gh-secret", body)

	// Valid request
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Invalid signature request
	reqBad := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	reqBad.Header.Set("X-GitHub-Event", "push")
	reqBad.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	wBad := httptest.NewRecorder()

	router.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wBad.Code)
	}
}

func TestHandler_GitLabWebhook(t *testing.T) {
	repo := NewMockRepository()
	repo.services["svc-gl"] = types.Service{
		ID:            "svc-gl",
		RepoURL:       "https://gitlab.com/acme/project.git",
		Branch:        "main",
		WebhookSecret: "gl-token-123",
	}

	svc := NewService(repo, ServiceConfig{})
	router := setupTestRouter(svc)

	payload := GitLabPushPayload{
		ObjectKind:  "push",
		CheckoutSHA: "commit456",
		Ref:         "refs/heads/main",
		Project: GitLabProjectPayload{
			GitHTTPURL: "https://gitlab.com/acme/project.git",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "gl-token-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ProtectedEndpoints(t *testing.T) {
	repo := NewMockRepository()
	mockGH := &MockGitClient{
		UserInfo: &types.GitProviderConnection{
			Username: "dev-octo",
		},
		Repositories: []types.GitRepository{
			{Name: "app-one", DefaultBranch: "main"},
		},
		Branches: []types.GitBranch{
			{Name: "main", CommitSHA: "aaa111"},
		},
	}
	svc := NewService(repo, ServiceConfig{
		PlatformURL: "http://localhost:8080",
	}, WithGitHubClient(mockGH))

	router := setupTestRouter(svc)

	token := createAuthToken(t, "user-99", "user99@adroit.run")

	// 1. Unauthenticated request to /git/providers should return 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/git/providers", nil)
	wUnauth := httptest.NewRecorder()
	router.ServeHTTP(wUnauth, unauthReq)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", wUnauth.Code)
	}

	// 2. Connect Provider
	connReqBody, _ := json.Marshal(ConnectProviderRequest{
		Provider:    types.GitProviderGitHub,
		AccessToken: "ghp_valid_mock_token",
	})
	connectReq := httptest.NewRequest(http.MethodPost, "/git/providers/connect", bytes.NewReader(connReqBody))
	connectReq.Header.Set("Authorization", "Bearer "+token)
	wConnect := httptest.NewRecorder()
	router.ServeHTTP(wConnect, connectReq)
	if wConnect.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wConnect.Code, wConnect.Body.String())
	}

	// 3. List Providers
	listReq := httptest.NewRequest(http.MethodGet, "/git/providers", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, listReq)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wList.Code, wList.Body.String())
	}

	// 4. List Repos
	reposReq := httptest.NewRequest(http.MethodGet, "/git/github/repos", nil)
	reposReq.Header.Set("Authorization", "Bearer "+token)
	wRepos := httptest.NewRecorder()
	router.ServeHTTP(wRepos, reposReq)
	if wRepos.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wRepos.Code, wRepos.Body.String())
	}

	// 5. List Branches
	branchReq := httptest.NewRequest(http.MethodGet, "/git/github/repos/dev-octo/app-one/branches", nil)
	branchReq.Header.Set("Authorization", "Bearer "+token)
	wBranch := httptest.NewRecorder()
	router.ServeHTTP(wBranch, branchReq)
	if wBranch.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wBranch.Code, wBranch.Body.String())
	}

	// 6. Setup Webhook for Service
	repo.services["svc-target"] = types.Service{
		ID:      "svc-target",
		RepoURL: "https://github.com/dev-octo/app-one.git",
		Branch:  "main",
	}

	hookReq := httptest.NewRequest(http.MethodPost, "/git/services/svc-target/webhook", nil)
	hookReq.Header.Set("Authorization", "Bearer "+token)
	wHook := httptest.NewRecorder()
	router.ServeHTTP(wHook, hookReq)
	if wHook.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wHook.Code, wHook.Body.String())
	}

	// 7. Get Webhook Info
	infoReq := httptest.NewRequest(http.MethodGet, "/git/services/svc-target/webhook", nil)
	infoReq.Header.Set("Authorization", "Bearer "+token)
	wInfo := httptest.NewRecorder()
	router.ServeHTTP(wInfo, infoReq)
	if wInfo.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wInfo.Code, wInfo.Body.String())
	}

	// 8. List Webhook Events
	eventsReq := httptest.NewRequest(http.MethodGet, "/git/webhooks/events", nil)
	eventsReq.Header.Set("Authorization", "Bearer "+token)
	wEvents := httptest.NewRecorder()
	router.ServeHTTP(wEvents, eventsReq)
	if wEvents.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wEvents.Code, wEvents.Body.String())
	}
}
