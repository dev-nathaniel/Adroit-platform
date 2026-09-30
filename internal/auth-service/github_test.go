package authservice_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authservice "github.com/dev-nathaniel/Adroit-platform/internal/auth-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func setupTestRouterWithGitHub(frontendURL string, ghClient authservice.GitHubClient) (chi.Router, string, *authservice.MemoryRepository) {
	jwtSecret := "test-suite-super-secret-key-32-bytes!"
	repo := authservice.NewMemoryRepository()

	var opts []authservice.Option
	if ghClient != nil {
		opts = append(opts, authservice.WithGitHubClient(ghClient))
	}
	if frontendURL != "" {
		opts = append(opts, authservice.WithFrontendURL(frontendURL))
	}

	svc := authservice.NewService(repo, jwtSecret, 1*time.Hour, opts...)
	handler := authservice.NewHandler(svc)

	r := chi.NewRouter()
	handler.RegisterRoutes(r, jwtSecret)

	return r, jwtSecret, repo
}

func TestGitHubAuthURL(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{}
	r, _, _ := setupTestRouterWithGitHub("", mockGH)

	// 1. Fetch URL without query state
	req := httptest.NewRequest(http.MethodGet, "/github/url", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res response.APIResponse[authservice.GitHubAuthURLResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Success || res.Data.URL == "" || res.Data.State == "" {
		t.Errorf("expected valid auth url and state, got: %+v", res.Data)
	}

	// 2. Fetch URL with explicit query state
	reqCustom := httptest.NewRequest(http.MethodGet, "/github/url?state=my-csrf-state", nil)
	recCustom := httptest.NewRecorder()
	r.ServeHTTP(recCustom, reqCustom)

	if recCustom.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with custom state, got %d", recCustom.Code)
	}

	var resCustom response.APIResponse[authservice.GitHubAuthURLResponse]
	_ = json.Unmarshal(recCustom.Body.Bytes(), &resCustom)
	if resCustom.Data.State != "my-csrf-state" {
		t.Errorf("expected state 'my-csrf-state', got %s", resCustom.Data.State)
	}
}

func TestGitHubLoginRedirect(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{}
	r, _, _ := setupTestRouterWithGitHub("", mockGH)

	req := httptest.NewRequest(http.MethodGet, "/github", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 Temporary Redirect, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize") {
		t.Errorf("expected redirect to GitHub authorize URL, got %s", loc)
	}

	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "oauth_state=") {
		t.Errorf("expected oauth_state cookie to be set, got: %s", cookie)
	}
}

func TestGitHubCallback_NewUserAndWorkspace(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{
		GetUserFunc: func(ctx context.Context, accessToken string) (*authservice.GitHubUserInfo, error) {
			return &authservice.GitHubUserInfo{
				ID:        1234567,
				Login:     "ghdeveloper",
				Name:      "GitHub Developer",
				Email:     "ghdeveloper@example.com",
				AvatarURL: "https://github.com/ghdeveloper.png",
			}, nil
		},
	}
	r, jwtSecret, repo := setupTestRouterWithGitHub("", mockGH)

	// Callback with code and state matching cookie
	req := httptest.NewRequest(http.MethodGet, "/github/callback?code=auth-code-123&state=state-abc", nil)
	req.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: "state-abc",
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on callback, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res response.APIResponse[authservice.AuthResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Success || res.Data.Token == "" {
		t.Fatalf("expected success and JWT token")
	}

	if res.Data.User.Email != "ghdeveloper@example.com" {
		t.Errorf("expected email ghdeveloper@example.com, got %s", res.Data.User.Email)
	}

	if res.Data.User.GitHubUsername != "ghdeveloper" {
		t.Errorf("expected GitHubUsername ghdeveloper, got %s", res.Data.User.GitHubUsername)
	}

	if res.Data.Organization == nil || res.Data.Organization.Name == "" {
		t.Errorf("expected default workspace to be provisioned for new GitHub user")
	}

	// Verify user is stored in repository and can be retrieved by GitHub ID
	account, err := repo.GetUserByGitHubID(context.Background(), "1234567")
	if err != nil || account == nil {
		t.Fatalf("expected user to be stored with github_id 1234567: %v", err)
	}

	// Verify JWT can access /me endpoint
	reqMe := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+res.Data.Token)
	recMe := httptest.NewRecorder()
	r.ServeHTTP(recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /me, got %d", recMe.Code)
	}
	_ = jwtSecret
}

func TestGitHubCallback_FrontendRedirect(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{
		GetUserFunc: func(ctx context.Context, accessToken string) (*authservice.GitHubUserInfo, error) {
			return &authservice.GitHubUserInfo{
				ID:        999111,
				Login:     "frontenduser",
				Name:      "Frontend User",
				Email:     "fe@example.com",
				AvatarURL: "",
			}, nil
		},
	}
	frontendURL := "http://localhost:3000/auth/callback"
	r, _, _ := setupTestRouterWithGitHub(frontendURL, mockGH)

	req := httptest.NewRequest(http.MethodGet, "/github/callback?code=frontend-code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect to frontend, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "http://localhost:3000/auth/callback?token=") {
		t.Errorf("expected redirect location with token query, got %s", location)
	}
}

func TestGitHubCallback_AccountLinking(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{
		GetUserFunc: func(ctx context.Context, accessToken string) (*authservice.GitHubUserInfo, error) {
			return &authservice.GitHubUserInfo{
				ID:        555666,
				Login:     "existinglink",
				Name:      "Link Account",
				Email:     "registered@adroit.run",
				AvatarURL: "https://avatars.github.com/u/555666",
			}, nil
		},
	}
	r, _, repo := setupTestRouterWithGitHub("", mockGH)

	// 1. First register user with standard email & password
	regBody, _ := json.Marshal(authservice.RegisterRequest{
		Email:    "registered@adroit.run",
		Name:     "Originally Registered",
		Password: "Password123!",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(regBody))
	recReg := httptest.NewRecorder()
	r.ServeHTTP(recReg, reqReg)

	if recReg.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on registration, got %d", recReg.Code)
	}

	// 2. Perform GitHub login with the same email
	reqGH := httptest.NewRequest(http.MethodGet, "/github/callback?code=link-code", nil)
	recGH := httptest.NewRecorder()
	r.ServeHTTP(recGH, reqGH)

	if recGH.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GitHub linking, got %d. Body: %s", recGH.Code, recGH.Body.String())
	}

	// 3. Verify user account was updated with GitHub identity
	acc, err := repo.GetUserByEmail(context.Background(), "registered@adroit.run")
	if err != nil {
		t.Fatalf("failed to retrieve user: %v", err)
	}
	if acc.GitHubID != "555666" || acc.GitHubUsername != "existinglink" {
		t.Errorf("expected linked github account details, got id=%s, user=%s", acc.GitHubID, acc.GitHubUsername)
	}

	// 4. User can still log in with their original password
	loginBody, _ := json.Marshal(authservice.LoginRequest{
		Email:    "registered@adroit.run",
		Password: "Password123!",
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	recLogin := httptest.NewRecorder()
	r.ServeHTTP(recLogin, reqLogin)

	if recLogin.Code != http.StatusOK {
		t.Errorf("expected password login to still work after GitHub link, got %d", recLogin.Code)
	}
}

func TestGitHubExchange_JSON(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{
		GetUserFunc: func(ctx context.Context, accessToken string) (*authservice.GitHubUserInfo, error) {
			return &authservice.GitHubUserInfo{
				ID:        777888,
				Login:     "spa-developer",
				Name:      "SPA Dev",
				Email:     "spa@adroit.run",
				AvatarURL: "https://avatars.github.com/u/777888",
			}, nil
		},
	}
	r, _, _ := setupTestRouterWithGitHub("", mockGH)

	payload := authservice.GitHubAuthRequest{
		Code: "spa-temp-code",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/github", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on POST /github, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res response.APIResponse[authservice.AuthResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Data.User.GitHubUsername != "spa-developer" {
		t.Errorf("expected spa-developer, got %s", res.Data.User.GitHubUsername)
	}
}

func TestGitHubAuth_Errors(t *testing.T) {
	mockGH := &authservice.MockGitHubClient{}
	r, _, _ := setupTestRouterWithGitHub("", mockGH)

	// 1. Missing code on callback
	reqNoCode := httptest.NewRequest(http.MethodGet, "/github/callback", nil)
	recNoCode := httptest.NewRecorder()
	r.ServeHTTP(recNoCode, reqNoCode)
	if recNoCode.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing code, got %d", recNoCode.Code)
	}

	// 2. GitHub returned error query parameter
	reqError := httptest.NewRequest(http.MethodGet, "/github/callback?error=access_denied&error_description=user_denied", nil)
	recError := httptest.NewRecorder()
	r.ServeHTTP(recError, reqError)
	if recError.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for github error, got %d", recError.Code)
	}

	// 3. Invalid state cookie mismatch
	reqStateMismatch := httptest.NewRequest(http.MethodGet, "/github/callback?code=some-code&state=forged-state", nil)
	reqStateMismatch.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: "legit-state",
	})
	recStateMismatch := httptest.NewRecorder()
	r.ServeHTTP(recStateMismatch, reqStateMismatch)
	if recStateMismatch.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for state mismatch, got %d", recStateMismatch.Code)
	}

	// 4. Invalid OAuth code
	reqBadCode := httptest.NewRequest(http.MethodGet, "/github/callback?code=invalid-code", nil)
	recBadCode := httptest.NewRecorder()
	r.ServeHTTP(recBadCode, reqBadCode)
	if recBadCode.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad verification code, got %d", recBadCode.Code)
	}

	// 5. Unconfigured GitHub client returns 501
	rUnconfigured, _, _ := setupTestRouterWithGitHub("", nil)
	reqUnconf := httptest.NewRequest(http.MethodGet, "/github/url", nil)
	recUnconf := httptest.NewRecorder()
	rUnconfigured.ServeHTTP(recUnconf, reqUnconf)
	if recUnconf.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 Not Implemented when GitHub unconfigured, got %d", recUnconf.Code)
	}
}
