package projectservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/identity"
	"github.com/dev-nathaniel/Adroit-platform/pkg/storage"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestRouter() (*chi.Mux, *MockRepository, string, string, string) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()
	store := storage.NewInMemStorage("https://storage.adroit.run/test-bucket")
	svc := NewService(repo, WithPublisher(bus), WithStorageProvider(store))
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

func TestHandlers_SourceUploadAndDownload(t *testing.T) {
	r, repo, _, _, token := setupTestRouter()
	orgID := "org_test_abc"

	// 1. Create a service with SourceTypeArchive
	createBody, _ := json.Marshal(CreateServiceRequest{
		OrganizationID: orgID,
		Name:           "Archive Handler Svc",
		SourceType:     types.SourceTypeArchive,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp struct {
		Success bool          `json:"success"`
		Data    types.Service `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	serviceID := createResp.Data.ID

	// 2. Prepare multipart zip upload request
	zipBytes := createSampleZip(t)
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "project.zip")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write(zipBytes)
	_ = writer.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/services/%s/upload", serviceID), body)
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()

	r.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for upload, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp struct {
		Success bool                 `json:"success"`
		Data    UploadSourceResponse `json:"data"`
	}
	if err := json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to parse upload response: %v", err)
	}

	if uploadResp.Data.SourceType != types.SourceTypeArchive {
		t.Errorf("expected source_type archive, got %s", uploadResp.Data.SourceType)
	}
	if uploadResp.Data.ArchiveURL == "" {
		t.Errorf("expected non-empty archive_url")
	}
	if uploadResp.Data.BuildID == "" {
		t.Errorf("expected non-empty build_id")
	}

	// 3. Test GET /api/v1/services/{id}/source/download
	downloadReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/services/%s/source/download", serviceID), nil)
	downloadReq.Header.Set("Authorization", "Bearer "+token)
	downloadRec := httptest.NewRecorder()

	r.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for download, got %d: %s", downloadRec.Code, downloadRec.Body.String())
	}

	contentType := downloadRec.Header().Get("Content-Type")
	if contentType != "application/gzip" {
		t.Errorf("expected Content-Type application/gzip, got %s", contentType)
	}
	contentDisp := downloadRec.Header().Get("Content-Disposition")
	if contentDisp != "attachment; filename=\"project.tar.gz\"" {
		t.Errorf("expected attachment Content-Disposition, got %s", contentDisp)
	}

	// 4. Test image-based service creation via API
	imageBody, _ := json.Marshal(CreateServiceRequest{
		OrganizationID: orgID,
		Name:           "Postgres Container",
		SourceType:     types.SourceTypeImage,
		ImageURL:       "postgres:16-alpine",
	})
	imgReq := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewReader(imageBody))
	imgReq.Header.Set("Authorization", "Bearer "+token)
	imgReq.Header.Set("Content-Type", "application/json")
	imgRec := httptest.NewRecorder()

	r.ServeHTTP(imgRec, imgReq)
	if imgRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for image service, got %d: %s", imgRec.Code, imgRec.Body.String())
	}

	var imgResp struct {
		Success bool          `json:"success"`
		Data    types.Service `json:"data"`
	}
	_ = json.Unmarshal(imgRec.Body.Bytes(), &imgResp)
	if imgResp.Data.SourceType != types.SourceTypeImage {
		t.Errorf("expected source_type image, got %s", imgResp.Data.SourceType)
	}
	if imgResp.Data.ImageURL != "postgres:16-alpine" {
		t.Errorf("expected image_url 'postgres:16-alpine', got '%s'", imgResp.Data.ImageURL)
	}
	if imgResp.Data.RepoURL != "" {
		t.Errorf("expected empty repo_url, got '%s'", imgResp.Data.RepoURL)
	}

	_ = repo
}
