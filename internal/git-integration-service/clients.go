package gitintegrationservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// GitClient abstracts interactions with a remote VCS provider (GitHub or GitLab)
type GitClient interface {
	GetUserInfo(ctx context.Context, token string) (*types.GitProviderConnection, error)
	ListRepositories(ctx context.Context, token string, page, perPage int) ([]types.GitRepository, error)
	ListBranches(ctx context.Context, token string, owner, repo string) ([]types.GitBranch, error)
	GetCommit(ctx context.Context, token string, owner, repo, ref string) (*types.GitCommitInfo, error)
	CreateWebhook(ctx context.Context, token string, owner, repo string, webhookURL, secret string) (string, error)
}

// ============================================================================
// GitHub Client
// ============================================================================

type GitHubClient struct {
	apiBaseURL string
	httpClient *http.Client
}

func NewGitHubClient(customBaseURL string) *GitHubClient {
	baseURL := "https://api.github.com"
	if customBaseURL != "" {
		baseURL = strings.TrimRight(customBaseURL, "/")
	}
	return &GitHubClient{
		apiBaseURL: baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *GitHubClient) GetUserInfo(ctx context.Context, token string) (*types.GitProviderConnection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("failed to decode github user: %w", err)
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	return &types.GitProviderConnection{
		Provider:    types.GitProviderGitHub,
		Username:    ghUser.Login,
		DisplayName: displayName,
		AvatarURL:   ghUser.AvatarURL,
	}, nil
}

func (c *GitHubClient) ListRepositories(ctx context.Context, token string, page, perPage int) ([]types.GitRepository, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	urlStr := fmt.Sprintf("%s/user/repos?sort=updated&per_page=%d&page=%d", c.apiBaseURL, perPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghRepos []struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
		HTMLURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		Description   string `json:"description"`
		UpdatedAt     string `json:"updated_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRepos); err != nil {
		return nil, fmt.Errorf("failed to decode github repositories: %w", err)
	}

	repos := make([]types.GitRepository, len(ghRepos))
	for i, r := range ghRepos {
		repos[i] = types.GitRepository{
			ID:            strconv.FormatInt(r.ID, 10),
			Name:          r.Name,
			FullName:      r.FullName,
			Owner:         r.Owner.Login,
			HTMLURL:       r.HTMLURL,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			DefaultBranch: r.DefaultBranch,
			IsPrivate:     r.Private,
			Description:   r.Description,
			UpdatedAt:     r.UpdatedAt,
		}
	}
	return repos, nil
}

func (c *GitHubClient) ListBranches(ctx context.Context, token string, owner, repo string) ([]types.GitBranch, error) {
	urlStr := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", c.apiBaseURL, url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghBranches []struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghBranches); err != nil {
		return nil, fmt.Errorf("failed to decode github branches: %w", err)
	}

	branches := make([]types.GitBranch, len(ghBranches))
	for i, b := range ghBranches {
		branches[i] = types.GitBranch{
			Name:      b.Name,
			CommitSHA: b.Commit.SHA,
			Protected: b.Protected,
		}
	}
	return branches, nil
}

func (c *GitHubClient) GetCommit(ctx context.Context, token string, owner, repo, ref string) (*types.GitCommitInfo, error) {
	urlStr := fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.apiBaseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghCommit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghCommit); err != nil {
		return nil, fmt.Errorf("failed to decode github commit: %w", err)
	}

	return &types.GitCommitInfo{
		SHA:       ghCommit.SHA,
		Message:   ghCommit.Commit.Message,
		Author:    ghCommit.Commit.Author.Name,
		Email:     ghCommit.Commit.Author.Email,
		Timestamp: ghCommit.Commit.Author.Date,
	}, nil
}

func (c *GitHubClient) CreateWebhook(ctx context.Context, token string, owner, repo string, webhookURL, secret string) (string, error) {
	urlStr := fmt.Sprintf("%s/repos/%s/%s/hooks", c.apiBaseURL, url.PathEscape(owner), url.PathEscape(repo))

	payload := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github webhook creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var hookResp struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hookResp); err != nil {
		return "", fmt.Errorf("failed to parse github webhook response: %w", err)
	}

	return strconv.FormatInt(hookResp.ID, 10), nil
}

// ============================================================================
// GitLab Client
// ============================================================================

type GitLabClient struct {
	apiBaseURL string
	httpClient *http.Client
}

func NewGitLabClient(customBaseURL string) *GitLabClient {
	baseURL := "https://gitlab.com"
	if customBaseURL != "" {
		baseURL = strings.TrimRight(customBaseURL, "/")
	}
	return &GitLabClient{
		apiBaseURL: baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *GitLabClient) GetUserInfo(ctx context.Context, token string) (*types.GitProviderConnection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/api/v4/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api returned status %d: %s", resp.StatusCode, string(body))
	}

	var glUser struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&glUser); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab user: %w", err)
	}

	displayName := glUser.Name
	if displayName == "" {
		displayName = glUser.Username
	}

	return &types.GitProviderConnection{
		Provider:    types.GitProviderGitLab,
		Username:    glUser.Username,
		DisplayName: displayName,
		AvatarURL:   glUser.AvatarURL,
		BaseURL:     c.apiBaseURL,
	}, nil
}

func (c *GitLabClient) ListRepositories(ctx context.Context, token string, page, perPage int) ([]types.GitRepository, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	urlStr := fmt.Sprintf("%s/api/v4/projects?membership=true&order_by=updated_at&per_page=%d&page=%d", c.apiBaseURL, perPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api returned status %d: %s", resp.StatusCode, string(body))
	}

	var glProjects []struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
		HTTPURLToRepo     string `json:"http_url_to_repo"`
		SSHURLToRepo      string `json:"ssh_url_to_repo"`
		DefaultBranch     string `json:"default_branch"`
		Visibility        string `json:"visibility"`
		Description       string `json:"description"`
		LastActivityAt    string `json:"last_activity_at"`
		Namespace         struct {
			FullPath string `json:"full_path"`
		} `json:"namespace"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&glProjects); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab projects: %w", err)
	}

	repos := make([]types.GitRepository, len(glProjects))
	for i, p := range glProjects {
		repos[i] = types.GitRepository{
			ID:            strconv.FormatInt(p.ID, 10),
			Name:          p.Name,
			FullName:      p.PathWithNamespace,
			Owner:         p.Namespace.FullPath,
			HTMLURL:       p.WebURL,
			CloneURL:      p.HTTPURLToRepo,
			SSHURL:        p.SSHURLToRepo,
			DefaultBranch: p.DefaultBranch,
			IsPrivate:     p.Visibility != "public",
			Description:   p.Description,
			UpdatedAt:     p.LastActivityAt,
		}
	}
	return repos, nil
}

func (c *GitLabClient) ListBranches(ctx context.Context, token string, owner, repo string) ([]types.GitBranch, error) {
	// GitLab project path is URL-escaped e.g. "group%2Fsubgroup%2Frepo"
	projectPath := owner + "/" + repo
	encodedPath := url.PathEscape(projectPath)

	urlStr := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=100", c.apiBaseURL, encodedPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api returned status %d: %s", resp.StatusCode, string(body))
	}

	var glBranches []struct {
		Name      string `json:"name"`
		Default   bool   `json:"default"`
		Protected bool   `json:"protected"`
		Commit    struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&glBranches); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab branches: %w", err)
	}

	branches := make([]types.GitBranch, len(glBranches))
	for i, b := range glBranches {
		branches[i] = types.GitBranch{
			Name:      b.Name,
			CommitSHA: b.Commit.ID,
			IsDefault: b.Default,
			Protected: b.Protected,
		}
	}
	return branches, nil
}

func (c *GitLabClient) GetCommit(ctx context.Context, token string, owner, repo, ref string) (*types.GitCommitInfo, error) {
	projectPath := owner + "/" + repo
	encodedPath := url.PathEscape(projectPath)

	urlStr := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits/%s", c.apiBaseURL, encodedPath, url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab api returned status %d: %s", resp.StatusCode, string(body))
	}

	var glCommit struct {
		ID            string    `json:"id"`
		Message       string    `json:"message"`
		AuthorName    string    `json:"author_name"`
		AuthorEmail   string    `json:"author_email"`
		CommittedDate time.Time `json:"committed_date"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&glCommit); err != nil {
		return nil, fmt.Errorf("failed to decode gitlab commit: %w", err)
	}

	return &types.GitCommitInfo{
		SHA:       glCommit.ID,
		Message:   glCommit.Message,
		Author:    glCommit.AuthorName,
		Email:     glCommit.AuthorEmail,
		Timestamp: glCommit.CommittedDate,
	}, nil
}

func (c *GitLabClient) CreateWebhook(ctx context.Context, token string, owner, repo string, webhookURL, secret string) (string, error) {
	projectPath := owner + "/" + repo
	encodedPath := url.PathEscape(projectPath)

	urlStr := fmt.Sprintf("%s/api/v4/projects/%s/hooks", c.apiBaseURL, encodedPath)

	payload := map[string]any{
		"url":                      webhookURL,
		"token":                    secret,
		"push_events":              true,
		"tag_push_events":          false,
		"merge_requests_events":    false,
		"enable_ssl_verification": true,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Adroit-Git-Integration")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gitlab api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gitlab webhook creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var hookResp struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hookResp); err != nil {
		return "", fmt.Errorf("failed to parse gitlab webhook response: %w", err)
	}

	return strconv.FormatInt(hookResp.ID, 10), nil
}
