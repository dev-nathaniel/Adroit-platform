package authservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GitHubUserInfo represents user details returned by GitHub's API
type GitHubUserInfo struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubConfig holds configuration parameters for GitHub OAuth
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	FrontendURL  string
	AuthURL      string // optional override, defaults to https://github.com/login/oauth/authorize
	TokenURL     string // optional override, defaults to https://github.com/login/oauth/access_token
	APIURL       string // optional override, defaults to https://api.github.com
}

// GitHubClient abstracts interactions with GitHub's OAuth & user endpoints
type GitHubClient interface {
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (string, error)
	GetUser(ctx context.Context, accessToken string) (*GitHubUserInfo, error)
}

// DefaultGitHubClient implements GitHubClient with HTTP requests to GitHub
type DefaultGitHubClient struct {
	cfg        GitHubConfig
	httpClient *http.Client
}

// NewDefaultGitHubClient creates a production-ready GitHub OAuth client
func NewDefaultGitHubClient(cfg GitHubConfig) *DefaultGitHubClient {
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.github.com"
	}
	return &DefaultGitHubClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetAuthURL builds the full authorization redirect URL
func (c *DefaultGitHubClient) GetAuthURL(state string) string {
	endpoint := c.cfg.AuthURL
	params := url.Values{}
	params.Set("client_id", c.cfg.ClientID)
	if c.cfg.RedirectURI != "" {
		params.Set("redirect_uri", c.cfg.RedirectURI)
	}
	params.Set("scope", "read:user user:email")
	if state != "" {
		params.Set("state", state)
	}
	return endpoint + "?" + params.Encode()
}

// ExchangeCode exchanges a temporary code from GitHub for an OAuth access token
func (c *DefaultGitHubClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	reqBody, err := json.Marshal(map[string]string{
		"client_id":     c.cfg.ClientID,
		"client_secret": c.cfg.ClientSecret,
		"code":          code,
		"redirect_uri":  c.cfg.RedirectURI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode exchange payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Adroit-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github token exchange returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenRes struct {
		AccessToken      string `json:"access_token"`
		TokenType        string `json:"token_type"`
		Scope            string `json:"scope"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return "", fmt.Errorf("failed to decode github token response: %w", err)
	}

	if tokenRes.Error != "" {
		return "", fmt.Errorf("github oauth error: %s (%s)", tokenRes.Error, tokenRes.ErrorDescription)
	}

	if tokenRes.AccessToken == "" {
		return "", errors.New("no access token received from github")
	}

	return tokenRes.AccessToken, nil
}

// GetUser retrieves the authenticated user's profile and verified email
func (c *DefaultGitHubClient) GetUser(ctx context.Context, accessToken string) (*GitHubUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ghUser GitHubUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("failed to decode github user response: %w", err)
	}

	// In case user profile has email hidden / set to private, fetch from /user/emails
	if ghUser.Email == "" {
		email, err := c.getPrimaryEmail(ctx, accessToken)
		if err == nil && email != "" {
			ghUser.Email = email
		}
	}

	// Fallback to noreply email if no public/private email found
	if ghUser.Email == "" {
		ghUser.Email = fmt.Sprintf("%s@users.noreply.github.com", ghUser.Login)
	}

	if ghUser.Name == "" {
		ghUser.Name = ghUser.Login
	}

	return &ghUser, nil
}

func (c *DefaultGitHubClient) getPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Adroit-Platform")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github emails API returned HTTP %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	// 1. Primary and verified
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	// 2. Any verified
	for _, e := range emails {
		if e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	// 3. First non-empty
	if len(emails) > 0 && emails[0].Email != "" {
		return emails[0].Email, nil
	}

	return "", errors.New("no verified email found in github account")
}

// MockGitHubClient provides an in-memory mock for unit tests and local dev
type MockGitHubClient struct {
	AuthURLFunc      func(state string) string
	ExchangeCodeFunc func(ctx context.Context, code string) (string, error)
	GetUserFunc      func(ctx context.Context, accessToken string) (*GitHubUserInfo, error)
}

func (m *MockGitHubClient) GetAuthURL(state string) string {
	if m.AuthURLFunc != nil {
		return m.AuthURLFunc(state)
	}
	return "https://github.com/login/oauth/authorize?client_id=mock-client-id&state=" + state
}

func (m *MockGitHubClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	if m.ExchangeCodeFunc != nil {
		return m.ExchangeCodeFunc(ctx, code)
	}
	if code == "invalid-code" {
		return "", errors.New("bad verification code")
	}
	return "mock-access-token-" + code, nil
}

func (m *MockGitHubClient) GetUser(ctx context.Context, accessToken string) (*GitHubUserInfo, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, accessToken)
	}
	return &GitHubUserInfo{
		ID:        98765432,
		Login:     "octocat",
		Name:      "The Octocat",
		Email:     "octocat@github.com",
		AvatarURL: "https://avatars.githubusercontent.com/u/98765432",
	}, nil
}
