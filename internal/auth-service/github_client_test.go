package authservice_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authservice "github.com/dev-nathaniel/Adroit-platform/internal/auth-service"
)

func TestDefaultGitHubClient_GetAuthURL(t *testing.T) {
	cfg := authservice.GitHubConfig{
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		RedirectURI:  "http://localhost:8080/api/v1/auth/github/callback",
		FrontendURL:  "http://localhost:3000",
	}
	client := authservice.NewDefaultGitHubClient(cfg)

	url := client.GetAuthURL("csrf-12345")
	if url != "https://github.com/login/oauth/authorize?client_id=my-client-id&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fapi%2Fv1%2Fauth%2Fgithub%2Fcallback&scope=read%3Auser+user%3Aemail&state=csrf-12345" {
		t.Errorf("unexpected auth URL: %s", url)
	}
}

func TestDefaultGitHubClient_ExchangeCodeAndGetUser(t *testing.T) {
	// Mock GitHub server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "gho_test_access_token_123",
				"token_type":   "bearer",
				"scope":        "read:user,user:email",
			})

		case "/user":
			if r.Header.Get("Authorization") != "Bearer gho_test_access_token_123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// Return profile with null/empty email to test /user/emails fallback
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         445566,
				"login":      "octotest",
				"name":       "Octo Test",
				"email":      "",
				"avatar_url": "https://avatars.githubusercontent.com/u/445566",
			})

		case "/user/emails":
			if r.Header.Get("Authorization") != "Bearer gho_test_access_token_123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"email":    "unverified@test.com",
					"primary":  false,
					"verified": false,
				},
				{
					"email":    "octotest-private@github.com",
					"primary":  true,
					"verified": true,
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	realClient := authservice.NewDefaultGitHubClient(authservice.GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "http://localhost:8080/callback",
		TokenURL:     ts.URL + "/login/oauth/access_token",
		APIURL:       ts.URL,
	})

	token, err := realClient.ExchangeCode(context.Background(), "good-code")
	if err != nil || token != "gho_test_access_token_123" {
		t.Fatalf("unexpected exchange: token=%s, err=%v", token, err)
	}

	user, err := realClient.GetUser(context.Background(), token)
	if err != nil || user.Email != "octotest-private@github.com" {
		t.Fatalf("unexpected user: user=%+v, err=%v", user, err)
	}
	if user.Name != "Octo Test" || user.Login != "octotest" {
		t.Errorf("unexpected user info: %+v", user)
	}
}
