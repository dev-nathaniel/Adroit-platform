package gitintegrationservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// ConnectProviderRequest holds inputs for connecting a Git provider account
type ConnectProviderRequest struct {
	Provider    types.GitProviderType `json:"provider"`              // "github" or "gitlab"
	AccessToken string                `json:"access_token"`          // Personal Access Token or OAuth Token
	BaseURL     string                `json:"base_url,omitempty"`    // Optional custom URL for self-hosted GitLab
}

// WebhookResult represents the outcome of a processed webhook event
type WebhookResult struct {
	Status          string   `json:"status"` // "ok", "ignored", "error"
	Provider        string   `json:"provider"`
	Event           string   `json:"event"`
	RepoCanonical   string   `json:"repo_canonical,omitempty"`
	Branch          string   `json:"branch,omitempty"`
	CommitHash      string   `json:"commit_hash,omitempty"`
	CommitMessage   string   `json:"commit_message,omitempty"`
	Author          string   `json:"author,omitempty"`
	MatchedServices []string `json:"matched_services"`
	BuildsTriggered []string `json:"builds_triggered"`
	Message         string   `json:"message,omitempty"`
}

// WebhookSetupResult represents the response when auto-configuring a webhook on GitHub/GitLab
type WebhookSetupResult struct {
	ServiceID     string `json:"service_id"`
	Provider      string `json:"provider"`
	WebhookID     string `json:"webhook_id,omitempty"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
	AutoConfigured bool   `json:"auto_configured"`
	Instructions  string `json:"instructions,omitempty"`
}

// ServiceWebhookInfo provides configuration instructions for manual webhook setup
type ServiceWebhookInfo struct {
	ServiceID     string `json:"service_id"`
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
	Instructions  string `json:"instructions"`
}

// GitHub Webhook Payload structures
type GitHubPushPayload struct {
	Ref        string                `json:"ref"` // "refs/heads/main"
	Before     string                `json:"before"`
	After      string                `json:"after"`
	Repository GitHubRepoPayload     `json:"repository"`
	Pusher     GitHubPusherPayload   `json:"pusher"`
	Sender     GitHubSenderPayload   `json:"sender"`
	HeadCommit *GitHubCommitPayload  `json:"head_commit"`
	Commits    []GitHubCommitPayload `json:"commits"`
}

type GitHubRepoPayload struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
}

type GitHubPusherPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type GitHubSenderPayload struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type GitHubCommitPayload struct {
	ID        string              `json:"id"`
	Message   string              `json:"message"`
	Timestamp string              `json:"timestamp"`
	Author    GitHubAuthorPayload `json:"author"`
}

type GitHubAuthorPayload struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// GitLab Webhook Payload structures
type GitLabPushPayload struct {
	ObjectKind   string                `json:"object_kind"`
	EventName    string                `json:"event_name"`
	Before       string                `json:"before"`
	After        string                `json:"after"`
	Ref          string                `json:"ref"` // "refs/heads/main"
	CheckoutSHA  string                `json:"checkout_sha"`
	UserName     string                `json:"user_name"`
	UserUsername string                `json:"user_username"`
	UserEmail    string                `json:"user_email"`
	ProjectID    int64                 `json:"project_id"`
	Project      GitLabProjectPayload  `json:"project"`
	Repository   GitLabRepoPayload     `json:"repository"`
	Commits      []GitLabCommitPayload `json:"commits"`
}

type GitLabProjectPayload struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	WebURL        string `json:"web_url"`
	GitSSHURL     string `json:"git_ssh_url"`
	GitHTTPURL    string `json:"git_http_url"`
	Namespace     string `json:"namespace"`
	DefaultBranch string `json:"default_branch"`
}

type GitLabRepoPayload struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	GitHTTPURL  string `json:"git_http_url"`
	GitSSHURL   string `json:"git_ssh_url"`
}

type GitLabCommitPayload struct {
	ID        string              `json:"id"`
	Message   string              `json:"message"`
	Title     string              `json:"title"`
	Timestamp string              `json:"timestamp"`
	Author    GitLabAuthorPayload `json:"author"`
}

type GitLabAuthorPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ParsedGitEvent is the provider-agnostic normalized representation of an incoming Git push event
type ParsedGitEvent struct {
	Provider      string
	EventType     string
	DeliveryID    string
	CloneURLs     []string
	Branch        string
	CommitHash    string
	CommitMessage string
	AuthorName    string
	AuthorEmail   string
	Timestamp     time.Time
}
