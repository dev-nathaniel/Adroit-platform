package buildservice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/builder"
	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

const testJWTSecret = "test-jwt-signing-secret-for-handlers-32-bytes"

func setupHandlerTestRouter(svc *Service) *chi.Mux {
	r := chi.NewRouter()
	h := NewHandler(svc)
	r.Route("/api/v1", func(api chi.Router) {
		h.RegisterRoutes(api, testJWTSecret)
	})
	return r
}

func createTestAuthToken(t *testing.T, userID, email string) string {
	token, err := identity.GenerateJWT(userID, email, testJWTSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return token
}

func TestHandler_TriggerBuild(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-100"
	orgID := "org-100"
	serviceID := "svc-100"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.orgSlugs[orgID] = "acme"
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "API",
		Slug:           "api",
		Branch:         "main",
		RepoURL:        "https://github.com/acme/api.git",
	}

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	body, _ := json.Marshal(TriggerBuildRequest{
		Branch:     "feat/payments",
		CommitHash: "deadbeefcafe",
		Message:    "Add payments API",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/"+serviceID+"/builds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var res response.APIResponse[types.Build]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Data.ServiceID != serviceID || res.Data.CommitHash != "deadbeefcafe" {
		t.Errorf("unexpected build response: %+v", res.Data)
	}
}

func TestHandler_TriggerBuildDirect(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-direct"
	orgID := "org-direct"
	serviceID := "svc-direct"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.orgSlugs[orgID] = "acme"
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
		Name:           "Web",
		Slug:           "web",
		Branch:         "main",
	}

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	body, _ := json.Marshal(map[string]any{
		"service_id":  serviceID,
		"branch":      "main",
		"commit_hash": "11223344",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/builds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ListServiceBuilds(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-list"
	orgID := "org-list"
	serviceID := "svc-list"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
	}

	ctx := context.Background()
	_ = repo.CreateBuild(ctx, &types.Build{
		ID:        "b-1",
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
	})
	_ = repo.CreateBuild(ctx, &types.Build{
		ID:        "b-2",
		ServiceID: serviceID,
		Status:    types.BuildStatusFailed,
	})

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/"+serviceID+"/builds?limit=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res response.APIResponse[BuildListResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Data.Total != 2 {
		t.Errorf("expected 2 total builds, got %d", res.Data.Total)
	}
}

func TestHandler_GetBuild(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-get"
	orgID := "org-get"
	serviceID := "svc-get"
	buildID := "build-get-123"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
	}
	_ = repo.CreateBuild(context.Background(), &types.Build{
		ID:         buildID,
		ServiceID:  serviceID,
		CommitHash: "abcdef",
		Status:     types.BuildStatusSucceeded,
		ImageTag:   "registry.adroit.run/acme/app:abcdef",
	})

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/builds/"+buildID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res response.APIResponse[types.Build]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if res.Data.ID != buildID || res.Data.ImageTag != "registry.adroit.run/acme/app:abcdef" {
		t.Errorf("unexpected build response: %+v", res.Data)
	}
}

func TestHandler_CancelBuild(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-cancel"
	orgID := "org-cancel"
	serviceID := "svc-cancel"
	buildID := "build-cancel-123"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
	}
	_ = repo.CreateBuild(context.Background(), &types.Build{
		ID:        buildID,
		ServiceID: serviceID,
		Status:    types.BuildStatusBuilding,
	})

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/builds/"+buildID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res response.APIResponse[types.Build]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if res.Data.Status != types.BuildStatusCancelled {
		t.Errorf("expected status cancelled, got %s", res.Data.Status)
	}
}

func TestHandler_GetBuildLogs(t *testing.T) {
	repo := NewMockRepository()
	userID := "user-logs"
	orgID := "org-logs"
	serviceID := "svc-logs"
	buildID := "build-logs-123"

	repo.orgMembers[orgID] = map[string]string{userID: "member"}
	repo.services[serviceID] = types.Service{
		ID:             serviceID,
		OrganizationID: orgID,
	}
	_ = repo.CreateBuild(context.Background(), &types.Build{
		ID:        buildID,
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
	})
	_ = repo.InsertBuildLog(context.Background(), &types.BuildLogEntry{
		BuildID: buildID,
		Line:    "=> Step 1 compile",
		Stream:  "stdout",
	})

	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)
	token := createTestAuthToken(t, userID, "user@acme.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/builds/"+buildID+"/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res response.APIResponse[BuildLogsResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if res.Data.TotalLines != 1 {
		t.Errorf("expected 1 log line, got %d", res.Data.TotalLines)
	}
}

func TestHandler_Unauthorized(t *testing.T) {
	repo := NewMockRepository()
	bld := builder.NewLocalDevBuilder(0)
	svc := NewService(repo, bld, ServiceConfig{})
	router := setupHandlerTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/builds/b-1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}
}
