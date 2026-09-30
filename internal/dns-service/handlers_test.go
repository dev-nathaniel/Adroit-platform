package dnsservice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestRouter() (*chi.Mux, *MockRepository, *MockDNSResolver, *Handler) {
	svc, repo, _, dnsResolver, _ := setupTestService()
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.RegisterRoutes(r, "") // Empty secret to disable auth middleware during handler tests

	return r, repo, dnsResolver, h
}

func TestHandler_Healthz(t *testing.T) {
	r, _, _, _ := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

func TestHandler_CreateAndListDomains(t *testing.T) {
	r, repo, _, _ := setupTestRouter()

	testSvc := &types.Service{
		ID:   "svc_test_http",
		Name: "HTTP Test Service",
		Slug: "http-test",
		Port: 8080,
	}
	repo.services[testSvc.ID] = testSvc

	// 1. Create Domain
	payload := CreateDomainRequest{
		Domain:           "frontend.acme.com",
		VerificationType: types.VerificationTypeCNAME,
		IsPrimary:        true,
	}
	data, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/services/"+testSvc.ID+"/domains", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", w.Code, w.Body.String())
	}

	var createResp struct {
		Success bool           `json:"success"`
		Data    DomainResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	domainID := createResp.Data.Domain.ID

	if domainID == "" {
		t.Fatal("expected non-empty domain ID")
	}

	// 2. List Domains
	listReq := httptest.NewRequest(http.MethodGet, "/services/"+testSvc.ID+"/domains", nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", listW.Code)
	}

	// 3. Get Single Domain
	getReq := httptest.NewRequest(http.MethodGet, "/domains/"+domainID, nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getW.Code)
	}

	// 4. Delete Domain
	delReq := httptest.NewRequest(http.MethodDelete, "/domains/"+domainID, nil)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", delW.Code)
	}
}

func TestHandler_VerifyAndSyncRoute(t *testing.T) {
	r, repo, dnsResolver, _ := setupTestRouter()

	testSvc := &types.Service{
		ID:   "svc_verify_test",
		Name: "Verification Svc",
		Slug: "verification-svc",
		Port: 8080,
	}
	repo.services[testSvc.ID] = testSvc

	// Seed domain
	d := &types.CustomDomain{
		ID:                "dom_verify_1",
		ServiceID:         testSvc.ID,
		Domain:            "portal.example.com",
		Status:            types.DomainStatusPendingVerification,
		VerificationType:  types.VerificationTypeCNAME,
		VerificationToken: "adr_verify_testtoken",
	}
	repo.domains[d.ID] = d

	// 1. Verify before DNS is setup -> StatusUnprocessableEntity (422)
	vReq := httptest.NewRequest(http.MethodPost, "/domains/"+d.ID+"/verify", nil)
	vW := httptest.NewRecorder()
	r.ServeHTTP(vW, vReq)

	if vW.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got %d", vW.Code)
	}

	// 2. Setup mock DNS to match CNAME target
	dnsResolver.SetCNAME("portal.example.com", "cname.adroit.run")

	vReq2 := httptest.NewRequest(http.MethodPost, "/domains/"+d.ID+"/verify", nil)
	vW2 := httptest.NewRecorder()
	r.ServeHTTP(vW2, vReq2)

	if vW2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", vW2.Code, vW2.Body.String())
	}

	// 3. Sync route with explicit upstream
	syncPayload := SyncRouteRequest{
		UpstreamURL: "10.0.0.10:8080",
	}
	syncData, _ := json.Marshal(syncPayload)
	sReq := httptest.NewRequest(http.MethodPost, "/domains/"+d.ID+"/sync", bytes.NewReader(syncData))
	sReq.Header.Set("Content-Type", "application/json")
	sW := httptest.NewRecorder()
	r.ServeHTTP(sW, sReq)

	if sW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", sW.Code, sW.Body.String())
	}

	// 4. List Ingress Routes
	routesReq := httptest.NewRequest(http.MethodGet, "/ingress/routes", nil)
	routesW := httptest.NewRecorder()
	r.ServeHTTP(routesW, routesReq)

	if routesW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ingress routes, got %d", routesW.Code)
	}
}
