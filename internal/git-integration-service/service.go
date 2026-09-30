package gitintegrationservice

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// ServiceConfig holds global configuration for git-integration-service
type ServiceConfig struct {
	GitHubWebhookSecret string
	GitLabWebhookSecret string
	PlatformURL         string // e.g. "http://localhost:8080" or "https://api.adroit.run"
}

// Service contains core business logic for VCS webhooks and integrations
type Service struct {
	repo         Repository
	eventBus     events.EventBus
	cfg          ServiceConfig
	githubClient GitClient
	gitlabClient GitClient
	logger       *slog.Logger
}

// ServiceOption allows functional configuration of Service
type ServiceOption func(*Service)

// WithEventBus sets the event bus
func WithEventBus(bus events.EventBus) ServiceOption {
	return func(s *Service) {
		s.eventBus = bus
	}
}

// WithGitHubClient sets a custom GitHub client (useful for mocks/tests)
func WithGitHubClient(cli GitClient) ServiceOption {
	return func(s *Service) {
		s.githubClient = cli
	}
}

// WithGitLabClient sets a custom GitLab client (useful for mocks/tests)
func WithGitLabClient(cli GitClient) ServiceOption {
	return func(s *Service) {
		s.gitlabClient = cli
	}
}

// WithLogger sets the structured logger
func WithLogger(log *slog.Logger) ServiceOption {
	return func(s *Service) {
		s.logger = log
	}
}

// NewService creates an initialized git integration service
func NewService(repo Repository, cfg ServiceConfig, opts ...ServiceOption) *Service {
	s := &Service{
		repo:         repo,
		cfg:          cfg,
		githubClient: NewGitHubClient(""),
		gitlabClient: NewGitLabClient(""),
		logger:       slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ============================================================================
// Webhook Processing
// ============================================================================

// ProcessGitHubWebhook handles incoming GitHub webhook payloads
func (s *Service) ProcessGitHubWebhook(ctx context.Context, deliveryID, eventType, signatureHeader string, body []byte) (*WebhookResult, error) {
	if eventType == "ping" {
		return &WebhookResult{
			Status:   "ok",
			Provider: "github",
			Event:    "ping",
			Message:  "GitHub webhook ping acknowledged successfully",
		}, nil
	}

	if eventType != "push" {
		return &WebhookResult{
			Status:   "ignored",
			Provider: "github",
			Event:    eventType,
			Message:  fmt.Sprintf("Event '%s' ignored (only 'push' triggers builds)", eventType),
		}, nil
	}

	var payload GitHubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse github push payload: %w", err)
	}

	// Determine repository clone URL
	rawRepoURL := payload.Repository.CloneURL
	if rawRepoURL == "" {
		rawRepoURL = payload.Repository.HTMLURL
	}
	if rawRepoURL == "" {
		rawRepoURL = payload.Repository.SSHURL
	}

	canonicalRepo, err := NormalizeGitURL(rawRepoURL)
	if err != nil {
		return nil, fmt.Errorf("invalid repository url in payload: %w", err)
	}

	branch := CleanBranchName(payload.Ref)
	commitHash := payload.After
	if commitHash == "" || commitHash == "0000000000000000000000000000000000000000" {
		if payload.HeadCommit != nil {
			commitHash = payload.HeadCommit.ID
		}
	}

	commitMessage := ""
	authorName := payload.Pusher.Name
	authorEmail := payload.Pusher.Email

	if payload.HeadCommit != nil {
		commitMessage = payload.HeadCommit.Message
		if payload.HeadCommit.Author.Name != "" {
			authorName = payload.HeadCommit.Author.Name
		}
		if payload.HeadCommit.Author.Email != "" {
			authorEmail = payload.HeadCommit.Author.Email
		}
	}

	// 1. Find matching services in database
	services, err := s.repo.FindMatchingServices(ctx, canonicalRepo, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup matching services: %w", err)
	}

	// 2. Validate webhook signature
	// If a global secret is set, verify against it
	// If services have custom secrets, verify against their secrets
	hasValidSecret := false
	if s.cfg.GitHubWebhookSecret != "" {
		if VerifyGitHubSignature(body, signatureHeader, s.cfg.GitHubWebhookSecret) {
			hasValidSecret = true
		}
	} else if len(services) > 0 {
		for _, svc := range services {
			if svc.WebhookSecret != "" && VerifyGitHubSignature(body, signatureHeader, svc.WebhookSecret) {
				hasValidSecret = true
				break
			}
		}
	}

	// If no secret configured anywhere, allow (dev mode)
	allEmpty := true
	if s.cfg.GitHubWebhookSecret == "" {
		for _, svc := range services {
			if svc.WebhookSecret != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty && signatureHeader == "" {
			hasValidSecret = true
		}
	} else {
		allEmpty = false
	}

	if !hasValidSecret && (signatureHeader != "" || !allEmpty) {
		return nil, ErrUnauthorized
	}

	// 3. Trigger builds for all matching services
	var matchedServiceIDs []string
	var triggeredBuildIDs []string

	for _, svc := range services {
		// If service has specific webhook secret, verify it
		if svc.WebhookSecret != "" && !VerifyGitHubSignature(body, signatureHeader, svc.WebhookSecret) && !VerifyGitHubSignature(body, signatureHeader, s.cfg.GitHubWebhookSecret) {
			continue
		}

		matchedServiceIDs = append(matchedServiceIDs, svc.ID)

		build := &types.Build{
			ServiceID:   svc.ID,
			CommitHash:  commitHash,
			Branch:      branch,
			Message:     commitMessage,
			Author:      authorName,
			Status:      types.BuildStatusQueued,
			TriggeredBy: "webhook:github",
			CreatedAt:   time.Now().UTC(),
		}

		if err := s.repo.CreateBuild(ctx, build); err != nil {
			s.logger.Error("Failed to create build record for webhook",
				slog.String("service_id", svc.ID),
				slog.String("commit", commitHash),
				slog.String("error", err.Error()),
			)
			continue
		}

		triggeredBuildIDs = append(triggeredBuildIDs, build.ID)

		// 4. Publish NATS JetStream Build Requested Event
		if s.eventBus != nil {
			projID := ""
			if svc.ProjectID != nil {
				projID = *svc.ProjectID
			}

			buildEvt := events.New(events.SubjectBuildRequested, "git-integration-service", events.BuildRequestedPayload{
				BuildID:        build.ID,
				ServiceID:      svc.ID,
				ProjectID:      projID,
				RepoURL:        svc.RepoURL,
				Branch:         svc.Branch,
				CommitHash:     commitHash,
				DockerfilePath: svc.DockerfilePath,
			})
			if err := s.eventBus.Publish(ctx, events.SubjectBuildRequested, buildEvt); err != nil {
				s.logger.Warn("Failed to publish SubjectBuildRequested event", slog.Any("error", err))
			}
		}
	}

	// 5. Emit GitPushReceived event
	if s.eventBus != nil && len(triggeredBuildIDs) > 0 {
		pushEvt := events.New(events.SubjectGitPushReceived, "git-integration-service", events.GitPushReceivedPayload{
			Provider:        "github",
			RepoURL:         rawRepoURL,
			Branch:          branch,
			CommitHash:      commitHash,
			CommitMessage:   commitMessage,
			AuthorName:      authorName,
			AuthorEmail:     authorEmail,
			MatchedServices: matchedServiceIDs,
			TriggeredBuilds: triggeredBuildIDs,
		})
		_ = s.eventBus.Publish(ctx, events.SubjectGitPushReceived, pushEvt)
	}

	// 6. Record audit log
	status := "processed"
	if len(matchedServiceIDs) == 0 {
		status = "ignored"
	}

	_ = s.repo.RecordWebhookEvent(ctx, &types.GitWebhookEventRecord{
		Provider:        "github",
		EventType:       eventType,
		DeliveryID:      deliveryID,
		RepoURL:         rawRepoURL,
		Branch:          branch,
		CommitHash:      commitHash,
		CommitMessage:   commitMessage,
		Author:          authorName,
		Status:          status,
		MatchedServices: matchedServiceIDs,
		TriggeredBuilds: triggeredBuildIDs,
		CreatedAt:       time.Now().UTC(),
	})

	return &WebhookResult{
		Status:          status,
		Provider:        "github",
		Event:           eventType,
		RepoCanonical:   canonicalRepo,
		Branch:          branch,
		CommitHash:      commitHash,
		CommitMessage:   commitMessage,
		Author:          authorName,
		MatchedServices: matchedServiceIDs,
		BuildsTriggered: triggeredBuildIDs,
		Message:         fmt.Sprintf("Processed push event for %s (%d builds triggered)", canonicalRepo, len(triggeredBuildIDs)),
	}, nil
}

// ProcessGitLabWebhook handles incoming GitLab webhook payloads
func (s *Service) ProcessGitLabWebhook(ctx context.Context, deliveryID, eventType, tokenHeader string, body []byte) (*WebhookResult, error) {
	// GitLab sends event header e.g. "Push Hook", "Tag Push Hook", "System Hook"
	if strings.EqualFold(eventType, "System Hook") || strings.EqualFold(eventType, "ping") {
		return &WebhookResult{
			Status:   "ok",
			Provider: "gitlab",
			Event:    eventType,
			Message:  "GitLab webhook ping acknowledged successfully",
		}, nil
	}

	var payload GitLabPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse gitlab payload: %w", err)
	}

	// Validate it's a push event
	if payload.ObjectKind != "push" && !strings.Contains(strings.ToLower(eventType), "push") {
		return &WebhookResult{
			Status:   "ignored",
			Provider: "gitlab",
			Event:    eventType,
			Message:  fmt.Sprintf("Event '%s' ignored (only 'push' triggers builds)", eventType),
		}, nil
	}

	rawRepoURL := payload.Project.GitHTTPURL
	if rawRepoURL == "" {
		rawRepoURL = payload.Project.WebURL
	}
	if rawRepoURL == "" {
		rawRepoURL = payload.Project.GitSSHURL
	}
	if rawRepoURL == "" {
		rawRepoURL = payload.Repository.GitHTTPURL
	}

	canonicalRepo, err := NormalizeGitURL(rawRepoURL)
	if err != nil {
		return nil, fmt.Errorf("invalid repository url in gitlab payload: %w", err)
	}

	branch := CleanBranchName(payload.Ref)
	commitHash := payload.CheckoutSHA
	if commitHash == "" || commitHash == "0000000000000000000000000000000000000000" {
		commitHash = payload.After
	}
	if (commitHash == "" || commitHash == "0000000000000000000000000000000000000000") && len(payload.Commits) > 0 {
		commitHash = payload.Commits[0].ID
	}

	commitMessage := ""
	authorName := payload.UserName
	authorEmail := payload.UserEmail

	if len(payload.Commits) > 0 {
		commitMessage = payload.Commits[0].Message
		if payload.Commits[0].Author.Name != "" {
			authorName = payload.Commits[0].Author.Name
		}
		if payload.Commits[0].Author.Email != "" {
			authorEmail = payload.Commits[0].Author.Email
		}
	}

	// 1. Find matching services in database
	services, err := s.repo.FindMatchingServices(ctx, canonicalRepo, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup matching services: %w", err)
	}

	// 2. Validate token
	hasValidToken := false
	if s.cfg.GitLabWebhookSecret != "" {
		if VerifyGitLabToken(tokenHeader, s.cfg.GitLabWebhookSecret) {
			hasValidToken = true
		}
	} else if len(services) > 0 {
		for _, svc := range services {
			if svc.WebhookSecret != "" && VerifyGitLabToken(tokenHeader, svc.WebhookSecret) {
				hasValidToken = true
				break
			}
		}
	}

	// If no secret configured anywhere, allow (dev mode)
	allEmpty := true
	if s.cfg.GitLabWebhookSecret == "" {
		for _, svc := range services {
			if svc.WebhookSecret != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty && tokenHeader == "" {
			hasValidToken = true
		}
	} else {
		allEmpty = false
	}

	if !hasValidToken && (tokenHeader != "" || !allEmpty) {
		return nil, ErrUnauthorized
	}

	// 3. Trigger builds for all matching services
	var matchedServiceIDs []string
	var triggeredBuildIDs []string

	for _, svc := range services {
		if svc.WebhookSecret != "" && !VerifyGitLabToken(tokenHeader, svc.WebhookSecret) && !VerifyGitLabToken(tokenHeader, s.cfg.GitLabWebhookSecret) {
			continue
		}

		matchedServiceIDs = append(matchedServiceIDs, svc.ID)

		build := &types.Build{
			ServiceID:   svc.ID,
			CommitHash:  commitHash,
			Branch:      branch,
			Message:     commitMessage,
			Author:      authorName,
			Status:      types.BuildStatusQueued,
			TriggeredBy: "webhook:gitlab",
			CreatedAt:   time.Now().UTC(),
		}

		if err := s.repo.CreateBuild(ctx, build); err != nil {
			s.logger.Error("Failed to create build record for gitlab webhook",
				slog.String("service_id", svc.ID),
				slog.String("commit", commitHash),
				slog.String("error", err.Error()),
			)
			continue
		}

		triggeredBuildIDs = append(triggeredBuildIDs, build.ID)

		// 4. Publish NATS JetStream Build Requested Event
		if s.eventBus != nil {
			projID := ""
			if svc.ProjectID != nil {
				projID = *svc.ProjectID
			}

			buildEvt := events.New(events.SubjectBuildRequested, "git-integration-service", events.BuildRequestedPayload{
				BuildID:        build.ID,
				ServiceID:      svc.ID,
				ProjectID:      projID,
				RepoURL:        svc.RepoURL,
				Branch:         svc.Branch,
				CommitHash:     commitHash,
				DockerfilePath: svc.DockerfilePath,
			})
			if err := s.eventBus.Publish(ctx, events.SubjectBuildRequested, buildEvt); err != nil {
				s.logger.Warn("Failed to publish SubjectBuildRequested event", slog.Any("error", err))
			}
		}
	}

	// 5. Emit GitPushReceived event
	if s.eventBus != nil && len(triggeredBuildIDs) > 0 {
		pushEvt := events.New(events.SubjectGitPushReceived, "git-integration-service", events.GitPushReceivedPayload{
			Provider:        "gitlab",
			RepoURL:         rawRepoURL,
			Branch:          branch,
			CommitHash:      commitHash,
			CommitMessage:   commitMessage,
			AuthorName:      authorName,
			AuthorEmail:     authorEmail,
			MatchedServices: matchedServiceIDs,
			TriggeredBuilds: triggeredBuildIDs,
		})
		_ = s.eventBus.Publish(ctx, events.SubjectGitPushReceived, pushEvt)
	}

	// 6. Record audit log
	status := "processed"
	if len(matchedServiceIDs) == 0 {
		status = "ignored"
	}

	_ = s.repo.RecordWebhookEvent(ctx, &types.GitWebhookEventRecord{
		Provider:        "gitlab",
		EventType:       eventType,
		DeliveryID:      deliveryID,
		RepoURL:         rawRepoURL,
		Branch:          branch,
		CommitHash:      commitHash,
		CommitMessage:   commitMessage,
		Author:          authorName,
		Status:          status,
		MatchedServices: matchedServiceIDs,
		TriggeredBuilds: triggeredBuildIDs,
		CreatedAt:       time.Now().UTC(),
	})

	return &WebhookResult{
		Status:          status,
		Provider:        "gitlab",
		Event:           eventType,
		RepoCanonical:   canonicalRepo,
		Branch:          branch,
		CommitHash:      commitHash,
		CommitMessage:   commitMessage,
		Author:          authorName,
		MatchedServices: matchedServiceIDs,
		BuildsTriggered: triggeredBuildIDs,
		Message:         fmt.Sprintf("Processed push event for %s (%d builds triggered)", canonicalRepo, len(triggeredBuildIDs)),
	}, nil
}

// ============================================================================
// Signature & Token Verification
// ============================================================================

// VerifyGitHubSignature validates the HMAC-SHA256 signature from GitHub
func VerifyGitHubSignature(payload []byte, signatureHeader, secret string) bool {
	if secret == "" {
		return true
	}
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	expectedMAC := signatureHeader[len("sha256="):]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualMAC := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(actualMAC), []byte(expectedMAC)) == 1
}

// VerifyGitLabToken validates the secret token from GitLab
func VerifyGitLabToken(tokenHeader, secret string) bool {
	if secret == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(secret)) == 1
}

// ============================================================================
// Provider Connection & Repository Discovery
// ============================================================================

// ConnectProvider links a personal access token or OAuth credential for GitHub or GitLab
func (s *Service) ConnectProvider(ctx context.Context, userID string, req ConnectProviderRequest) (*types.GitProviderConnection, error) {
	if req.AccessToken == "" {
		return nil, errors.New("access_token is required")
	}

	var client GitClient
	switch req.Provider {
	case types.GitProviderGitHub:
		client = s.githubClient
	case types.GitProviderGitLab:
		if req.BaseURL != "" {
			client = NewGitLabClient(req.BaseURL)
		} else {
			client = s.gitlabClient
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s (must be 'github' or 'gitlab')", req.Provider)
	}

	// Verify token with remote provider
	userConn, err := client.GetUserInfo(ctx, req.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with %s: %w", req.Provider, err)
	}

	userConn.UserID = userID
	userConn.Provider = req.Provider
	userConn.BaseURL = req.BaseURL

	if err := s.repo.SaveGitProvider(ctx, userConn, req.AccessToken); err != nil {
		return nil, fmt.Errorf("failed to save git provider connection: %w", err)
	}

	return userConn, nil
}

// ListProviders returns all connected VCS providers for a user
func (s *Service) ListProviders(ctx context.Context, userID string) ([]types.GitProviderConnection, error) {
	return s.repo.ListGitProviders(ctx, userID)
}

// DisconnectProvider unlinks a provider account
func (s *Service) DisconnectProvider(ctx context.Context, userID string, provider types.GitProviderType) error {
	return s.repo.DeleteGitProvider(ctx, userID, provider)
}

// ListRepositories retrieves repositories from the user's connected provider
func (s *Service) ListRepositories(ctx context.Context, userID string, provider types.GitProviderType, page, perPage int) ([]types.GitRepository, error) {
	conn, token, err := s.repo.GetGitProvider(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("git provider %s not connected for this user: %w", provider, err)
	}

	var client GitClient
	switch provider {
	case types.GitProviderGitHub:
		client = s.githubClient
	case types.GitProviderGitLab:
		if conn.BaseURL != "" {
			client = NewGitLabClient(conn.BaseURL)
		} else {
			client = s.gitlabClient
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	return client.ListRepositories(ctx, token, page, perPage)
}

// ListBranches retrieves branches for a specific repository
func (s *Service) ListBranches(ctx context.Context, userID string, provider types.GitProviderType, owner, repo string) ([]types.GitBranch, error) {
	conn, token, err := s.repo.GetGitProvider(ctx, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("git provider %s not connected: %w", provider, err)
	}

	var client GitClient
	switch provider {
	case types.GitProviderGitHub:
		client = s.githubClient
	case types.GitProviderGitLab:
		if conn.BaseURL != "" {
			client = NewGitLabClient(conn.BaseURL)
		} else {
			client = s.gitlabClient
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	return client.ListBranches(ctx, token, owner, repo)
}

// SetupServiceWebhook generates a per-service webhook secret, stores it, and attempts
// to auto-register the webhook on GitHub or GitLab if an access token is available
func (s *Service) SetupServiceWebhook(ctx context.Context, userID, serviceID string) (*WebhookSetupResult, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	host, owner, repoName, err := ExtractRepoDetails(svc.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse service repo URL: %w", err)
	}

	// Generate secret if not present
	secret := svc.WebhookSecret
	if secret == "" {
		secBytes := make([]byte, 16)
		_, _ = rand.Read(secBytes)
		secret = "whsec_" + hex.EncodeToString(secBytes)
		if err := s.repo.UpdateServiceWebhookSecret(ctx, svc.ID, secret); err != nil {
			return nil, fmt.Errorf("failed to save webhook secret: %w", err)
		}
	}

	baseURL := s.cfg.PlatformURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var provider types.GitProviderType
	var webhookURL string
	var webhookEndpoint string

	if strings.Contains(host, "gitlab") {
		provider = types.GitProviderGitLab
		webhookURL = baseURL + "/api/v1/webhooks/gitlab"
		webhookEndpoint = "GitLab -> Project Settings -> Webhooks"
	} else {
		provider = types.GitProviderGitHub
		webhookURL = baseURL + "/api/v1/webhooks/github"
		webhookEndpoint = "GitHub -> Repository Settings -> Webhooks"
	}

	res := &WebhookSetupResult{
		ServiceID:      svc.ID,
		Provider:       string(provider),
		WebhookURL:     webhookURL,
		WebhookSecret:  secret,
		AutoConfigured: false,
		Instructions:   fmt.Sprintf("Add a webhook in %s with Payload URL: %s, Content type: application/json, and Secret / Token: %s", webhookEndpoint, webhookURL, secret),
	}

	// Attempt automatic registration if user has connected credentials
	conn, token, err := s.repo.GetGitProvider(ctx, userID, provider)
	if err == nil && token != "" {
		var client GitClient
		if provider == types.GitProviderGitHub {
			client = s.githubClient
		} else {
			if conn.BaseURL != "" {
				client = NewGitLabClient(conn.BaseURL)
			} else {
				client = s.gitlabClient
			}
		}

		hookID, err := client.CreateWebhook(ctx, token, owner, repoName, webhookURL, secret)
		if err == nil {
			res.AutoConfigured = true
			res.WebhookID = hookID
			res.Instructions = fmt.Sprintf("Webhook automatically created on %s (Hook ID: %s)", provider, hookID)
		} else {
			s.logger.Warn("Failed to auto-create webhook on provider, manual instructions provided",
				slog.String("provider", string(provider)),
				slog.String("error", err.Error()),
			)
		}
	}

	return res, nil
}

// GetServiceWebhookInfo returns webhook setup info and instructions for a service
func (s *Service) GetServiceWebhookInfo(ctx context.Context, serviceID string) (*ServiceWebhookInfo, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	host, _, _, _ := ExtractRepoDetails(svc.RepoURL)

	baseURL := s.cfg.PlatformURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var webhookURL string
	var providerDesc string
	if strings.Contains(host, "gitlab") {
		webhookURL = baseURL + "/api/v1/webhooks/gitlab"
		providerDesc = "GitLab (Settings -> Webhooks -> Add new webhook. Trigger: Push events)"
	} else {
		webhookURL = baseURL + "/api/v1/webhooks/github"
		providerDesc = "GitHub (Settings -> Webhooks -> Add webhook. Events: Just the push event)"
	}

	return &ServiceWebhookInfo{
		ServiceID:     svc.ID,
		RepoURL:       svc.RepoURL,
		Branch:        svc.Branch,
		WebhookURL:    webhookURL,
		WebhookSecret: svc.WebhookSecret,
		Instructions:  fmt.Sprintf("Configure webhook in %s. Set Payload URL to '%s', Content type to 'application/json', and Secret to '%s'.", providerDesc, webhookURL, svc.WebhookSecret),
	}, nil
}

// ListRecentWebhookEvents returns the audit trail of recently received webhooks
func (s *Service) ListRecentWebhookEvents(ctx context.Context, limit int) ([]types.GitWebhookEventRecord, error) {
	return s.repo.ListWebhookEvents(ctx, limit)
}
