package projectservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestRouter() (*chi.Mux, *MockRepository, string, string, string) {
	repo := NewMockRepository()
	svc := NewService(repo)
	handler := NewHandler(svc)

	jwtSecret := "test-jwt-secret-key-32-chars-long!"
	userID := "usr_test_123"
	orgID := "org_test_abc"

	repo.AddMember(orgID, userID, "owner")

	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api, jwtSecret)
	})

	token, _ := identity.GenerateJWT(userID, "test@example.com", jwtSecret, time.Hour)
	return r, repo, jwtSecret, userID, token
}

func TestHandlers_ProjectsAndServices(t *testing.T) {
	r, repo, _, _, token := setupTestRouter()
	orgID := "org_test_abc"

	var projectID string
	var serviceID string

	t.Run("POST /api/v1/projects - create project", func(t *testing.T) {
		body, _ := json.Marshal(CreateProjectRequest{
			OrganizationID: orgID,
			Name:           "Core Services",
			Description:    "Primary microservices",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Success bool          `json:"success"`
			Data    types.Project `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Data.ID == "" {
			t.Errorf("expected non-empty project ID")
		}
		projectID = resp.Data.ID
	})

	t.Run("POST /api/v1/services - create standalone service", func(t *testing.T) {
		body, _ := json.Marshal(CreateServiceRequest{
			OrganizationID: orgID,
			ProjectID:      nil, // Standalone!
			Name:           "Standalone Worker",
			RepoURL:        "https://github.com/example/standalone-worker",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Success bool          `json:"success"`
			Data    types.Service `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Data.ProjectID != nil {
			t.Errorf("expected nil project_id for standalone service, got %v", *resp.Data.ProjectID)
		}
		serviceID = resp.Data.ID
	})

	t.Run("POST /api/v1/services/{id}/move - move standalone service into project", func(t *testing.T) {
		body, _ := json.Marshal(MoveServiceRequest{
			ProjectID: &projectID,
		})
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/services/%s/move", serviceID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Success bool          `json:"success"`
			Data    types.Service `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Data.ProjectID == nil || *resp.Data.ProjectID != projectID {
			t.Errorf("expected service to be moved into project %s, got %v", projectID, resp.Data.ProjectID)
		}
	})

	t.Run("POST /api/v1/services/{id}/move - detach service back to standalone", func(t *testing.T) {
		body, _ := json.Marshal(MoveServiceRequest{
			ProjectID: nil, // Detach!
		})
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/services/%s/move", serviceID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Success bool          `json:"success"`
			Data    types.Service `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Data.ProjectID != nil {
			t.Errorf("expected service to be standalone (project_id=nil), got %v", *resp.Data.ProjectID)
		}
	})

	t.Run("POST /api/v1/services/{id}/env - set and get environment variables", func(t *testing.T) {
		body, _ := json.Marshal(SetEnvVarsRequest{
			Variables: []EnvVarItem{
				{Key: "APP_ENV", Value: "staging", IsSecret: false},
				{Key: "API_KEY", Value: "secret_123", IsSecret: true},
			},
		})
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/services/%s/env", serviceID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		// GET service env vars
		getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/services/%s/env", serviceID), nil)
		getReq.Header.Set("Authorization", "Bearer "+token)
		getRec := httptest.NewRecorder()

		r.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", getRec.Code, getRec.Body.String())
		}
	})

	t.Run("DELETE /api/v1/services/{id} - delete service", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/services/%s", serviceID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		// Confirm deleted
		if _, err := repo.GetServiceByID(context.Background(), serviceID); err == nil {
			t.Errorf("expected service to be deleted")
		}
	})
}
