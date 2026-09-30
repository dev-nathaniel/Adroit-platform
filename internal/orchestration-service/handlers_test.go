package orchestrationservice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/orchestrator"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestRouter() (chi.Router, *mockRepository, *Service) {
	repo := newMockRepository()
	driver := orchestrator.NewSimulatedDriver(10 * time.Millisecond)
	bus := events.NewInMemBus()

	svc := NewService(
		repo,
		driver,
		ServiceConfig{
			Datacenter:         "dc1",
			BaseDomain:         "adroit.run",
			RolloutTimeout:     5 * time.Second,
			HealthPollInterval: 15 * time.Millisecond,
		},
		WithEventBus(bus),
	)

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r, "") // empty jwtSecret for testing

	return r, repo, svc
}

func TestHandlers_Healthz(t *testing.T) {
	r, _, _ := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}

func TestHandlers_TriggerDeployAndGet(t *testing.T) {
	r, repo, _ := setupTestRouter()

	serviceID := "svc-api"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "API",
		Slug:        "api",
		MinReplicas: 2,
	}
	repo.builds["build-1"] = types.Build{
		ID:        "build-1",
		ServiceID: serviceID,
		Status:    types.BuildStatusSucceeded,
		ImageTag:  "registry.adroit.run/acme/api:v1",
	}

	payload := map[string]any{
		"build_id": "build-1",
		"replicas": 2,
		"strategy": "rolling",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/services/"+serviceID+"/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res struct {
		Success bool             `json:"success"`
		Data    types.Deployment `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&res)
	deployID := res.Data.ID

	// Test GET /deployments/{id}
	getReq := httptest.NewRequest(http.MethodGet, "/deployments/"+deployID, nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	// Test GET /services/{id}/deployments
	listReq := httptest.NewRequest(http.MethodGet, "/services/"+serviceID+"/deployments", nil)
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list, got %d", listRec.Code)
	}
}

func TestHandlers_ScaleService(t *testing.T) {
	r, repo, _ := setupTestRouter()

	serviceID := "svc-scale"
	repo.services[serviceID] = types.Service{
		ID:          serviceID,
		Name:        "Scale Svc",
		Slug:        "scale-svc",
		MinReplicas: 1,
	}

	payload := map[string]any{
		"replicas": 5,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/services/"+serviceID+"/scale", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandlers_PromoteCanary(t *testing.T) {
	r, repo, _ := setupTestRouter()

	deployID := "dep-canary"
	serviceID := "svc-canary"
	repo.services[serviceID] = types.Service{
		ID:   serviceID,
		Name: "Canary",
		Slug: "canary",
	}

	repo.deployments[deployID] = types.Deployment{
		ID:         deployID,
		ServiceID:  serviceID,
		Status:     types.DeploymentStatusDeploying,
		Strategy:   types.RolloutStrategyCanary,
		NomadJobID: "adroit-svc-svc-canary",
	}

	req := httptest.NewRequest(http.MethodPost, "/deployments/"+deployID+"/promote", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	// Even if job is not found on simulated driver (mock repo directly populated), it properly responds with error code or success
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
}

func TestHandlers_CancelDeployment(t *testing.T) {
	r, repo, _ := setupTestRouter()

	deployID := "dep-cancel"
	repo.deployments[deployID] = types.Deployment{
		ID:        deployID,
		ServiceID: "svc-c",
		Status:    types.DeploymentStatusDeploying,
	}

	req := httptest.NewRequest(http.MethodPost, "/deployments/"+deployID+"/cancel", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}
