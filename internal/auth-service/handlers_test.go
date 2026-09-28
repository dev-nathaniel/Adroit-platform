package authservice_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authservice "github.com/dev-nathaniel/Adroit-platform/internal/auth-service"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func setupTestRouter() (chi.Router, string) {
	jwtSecret := "test-suite-super-secret-key-32-bytes!"
	repo := authservice.NewMemoryRepository()
	svc := authservice.NewService(repo, jwtSecret, 1*time.Hour)
	handler := authservice.NewHandler(svc)

	r := chi.NewRouter()
	handler.RegisterRoutes(r, jwtSecret)

	return r, jwtSecret
}

func TestAuthRegisterAndLoginFlow(t *testing.T) {
	r, _ := setupTestRouter()

	// 1. Register a new user
	regPayload := authservice.RegisterRequest{
		Email:    "founder@adroit.run",
		Name:     "Platform Founder",
		Password: "SecurePassword123!",
	}
	body, _ := json.Marshal(regPayload)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on register, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var res response.APIResponse[authservice.AuthResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}

	if !res.Success || res.Data.Token == "" {
		t.Fatalf("expected success true and non-empty token")
	}

	if res.Data.Organization == nil || res.Data.Organization.Name == "" {
		t.Errorf("expected default organization to be provisioned")
	}

	token := res.Data.Token

	// 2. Register with duplicate email returns 409 Conflict
	recConflict := httptest.NewRecorder()
	reqConflict := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	r.ServeHTTP(recConflict, reqConflict)

	if recConflict.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict on duplicate email, got %d", recConflict.Code)
	}

	// 3. Login with correct password
	loginPayload := authservice.LoginRequest{
		Email:    "founder@adroit.run",
		Password: "SecurePassword123!",
	}
	loginBody, _ := json.Marshal(loginPayload)

	recLogin := httptest.NewRecorder()
	reqLogin := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	r.ServeHTTP(recLogin, reqLogin)

	if recLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on login, got %d", recLogin.Code)
	}

	// 4. Login with wrong password returns 401
	wrongLoginPayload := authservice.LoginRequest{
		Email:    "founder@adroit.run",
		Password: "IncorrectPassword!",
	}
	wrongBody, _ := json.Marshal(wrongLoginPayload)

	recWrong := httptest.NewRecorder()
	reqWrong := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(wrongBody))
	r.ServeHTTP(recWrong, reqWrong)

	if recWrong.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on wrong password, got %d", recWrong.Code)
	}

	// 5. Access protected /me without token returns 401
	recMeNoAuth := httptest.NewRecorder()
	reqMeNoAuth := httptest.NewRequest(http.MethodGet, "/me", nil)
	r.ServeHTTP(recMeNoAuth, reqMeNoAuth)

	if recMeNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on /me without auth, got %d", recMeNoAuth.Code)
	}

	// 6. Access protected /me with valid token returns 200 + profile
	recMe := httptest.NewRecorder()
	reqMe := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("expected 200 on /me with valid token, got %d", recMe.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	r, _ := setupTestRouter()

	// Short password (< 8 chars)
	payload := authservice.RegisterRequest{
		Email:    "valid@adroit.run",
		Password: "short",
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", rec.Code)
	}

	// Invalid email
	badEmailPayload := authservice.RegisterRequest{
		Email:    "not-an-email",
		Password: "validPassword123!",
	}
	badEmailBody, _ := json.Marshal(badEmailPayload)
	recBadEmail := httptest.NewRecorder()
	reqBadEmail := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(badEmailBody))
	r.ServeHTTP(recBadEmail, reqBadEmail)

	if recBadEmail.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid email, got %d", recBadEmail.Code)
	}
}
