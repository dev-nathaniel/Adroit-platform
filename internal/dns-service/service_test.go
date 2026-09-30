package dnsservice

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/ingress"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestService() (*Service, *MockRepository, *ingress.SimulatedIngressDriver, *MockDNSResolver, *events.InMemBus) {
	repo := NewMockRepository()
	ingressDriver := ingress.NewSimulatedIngressDriver()
	dnsResolver := NewMockDNSResolver()
	bus := events.NewInMemBus()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewService(repo, ingressDriver, dnsResolver, bus, logger, ServiceConfig{
		PlatformDomain: "adroit.run",
		CanonicalCNAME: "cname.adroit.run",
	})

	return svc, repo, ingressDriver, dnsResolver, bus
}

func TestAddCustomDomain(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, _ := setupTestService()

	// Seed test service
	testSvc := &types.Service{
		ID:   "svc_123",
		Name: "API Gateway",
		Slug: "api-gateway",
		Port: 8080,
	}
	repo.services[testSvc.ID] = testSvc

	t.Run("Valid CNAME Domain Creation", func(t *testing.T) {
		req := CreateDomainRequest{
			Domain:           "api.acme.corp",
			VerificationType: types.VerificationTypeCNAME,
			IsPrimary:        true,
		}

		resp, err := svc.AddCustomDomain(ctx, testSvc.ID, req)
		if err != nil {
			t.Fatalf("AddCustomDomain failed: %v", err)
		}

		if resp.Domain.Domain != "api.acme.corp" {
			t.Errorf("expected domain api.acme.corp, got %s", resp.Domain.Domain)
		}
		if resp.Domain.Status != types.DomainStatusPendingVerification {
			t.Errorf("expected status pending_verification, got %s", resp.Domain.Status)
		}
		if resp.Domain.VerificationToken == "" {
			t.Error("expected non-empty verification token")
		}
		if resp.ChallengeDetails.TargetHost != "cname.adroit.run" {
			t.Errorf("expected target host cname.adroit.run, got %s", resp.ChallengeDetails.TargetHost)
		}
	})

	t.Run("Invalid Domain Syntax", func(t *testing.T) {
		req := CreateDomainRequest{
			Domain: "invalid_domain",
		}
		_, err := svc.AddCustomDomain(ctx, testSvc.ID, req)
		if !errors.Is(err, ErrInvalidDomainName) {
			t.Errorf("expected ErrInvalidDomainName, got %v", err)
		}
	})

	t.Run("Duplicate Domain Conflict", func(t *testing.T) {
		req := CreateDomainRequest{Domain: "api.acme.corp"}
		_, err := svc.AddCustomDomain(ctx, testSvc.ID, req)
		if !errors.Is(err, ErrDomainAlreadyExists) {
			t.Errorf("expected ErrDomainAlreadyExists, got %v", err)
		}
	})

	t.Run("Non-existent Service", func(t *testing.T) {
		req := CreateDomainRequest{Domain: "other.domain.com"}
		_, err := svc.AddCustomDomain(ctx, "nonexistent", req)
		if !errors.Is(err, ErrServiceNotFound) {
			t.Errorf("expected ErrServiceNotFound, got %v", err)
		}
	})
}

func TestVerifyCustomDomain(t *testing.T) {
	ctx := context.Background()
	svc, repo, ingressDriver, dnsResolver, _ := setupTestService()

	testSvc := &types.Service{
		ID:   "svc_456",
		Name: "Storefront",
		Slug: "storefront",
		Port: 3000,
	}
	repo.services[testSvc.ID] = testSvc

	t.Run("CNAME Verification Success and Auto-Route Sync", func(t *testing.T) {
		// Active deployment running on node-1:24000
		repo.deployments["dep_active"] = &types.Deployment{
			ID:        "dep_active",
			ServiceID: testSvc.ID,
			Status:    types.DeploymentStatusActive,
			ActiveURL: "http://10.0.0.5:24000",
		}

		addResp, err := svc.AddCustomDomain(ctx, testSvc.ID, CreateDomainRequest{
			Domain:           "store.example.com",
			VerificationType: types.VerificationTypeCNAME,
		})
		if err != nil {
			t.Fatalf("AddCustomDomain failed: %v", err)
		}

		// Configure mock DNS to resolve store.example.com -> cname.adroit.run
		dnsResolver.SetCNAME("store.example.com", "cname.adroit.run")

		verifyResp, err := svc.VerifyCustomDomain(ctx, addResp.Domain.ID)
		if err != nil {
			t.Fatalf("VerifyCustomDomain failed: %v", err)
		}

		if !verifyResp.Verified {
			t.Errorf("expected verified=true, message: %s", verifyResp.Message)
		}
		if verifyResp.Status != types.DomainStatusActive {
			t.Errorf("expected status active, got %s", verifyResp.Status)
		}

		// Verify route was auto-registered in Ingress driver
		routeID := ingress.GenerateRouteID("store.example.com")
		r, err := ingressDriver.GetRoute(ctx, routeID)
		if err != nil {
			t.Fatalf("expected route in ingress driver: %v", err)
		}
		if len(r.Upstreams) == 0 || r.Upstreams[0].Dial != "10.0.0.5:24000" {
			t.Errorf("unexpected upstreams: %+v", r.Upstreams)
		}
	})

	t.Run("TXT Challenge Verification Success", func(t *testing.T) {
		addResp, err := svc.AddCustomDomain(ctx, testSvc.ID, CreateDomainRequest{
			Domain:           "secure.example.com",
			VerificationType: types.VerificationTypeTXT,
		})
		if err != nil {
			t.Fatalf("AddCustomDomain failed: %v", err)
		}

		// Set mock TXT record matching verification token
		recordName := "_adroit-challenge.secure.example.com"
		dnsResolver.SetTXT(recordName, []string{addResp.Domain.VerificationToken})

		verifyResp, err := svc.VerifyCustomDomain(ctx, addResp.Domain.ID)
		if err != nil {
			t.Fatalf("VerifyCustomDomain failed: %v", err)
		}
		if !verifyResp.Verified {
			t.Errorf("expected TXT challenge to pass, got: %s", verifyResp.Message)
		}
	})

	t.Run("CNAME Verification Failure on Wrong Target", func(t *testing.T) {
		addResp, _ := svc.AddCustomDomain(ctx, testSvc.ID, CreateDomainRequest{
			Domain:           "wrong.example.com",
			VerificationType: types.VerificationTypeCNAME,
		})

		dnsResolver.SetCNAME("wrong.example.com", "somewhere-else.net")

		verifyResp, err := svc.VerifyCustomDomain(ctx, addResp.Domain.ID)
		if err != nil {
			t.Fatalf("VerifyCustomDomain failed: %v", err)
		}
		if verifyResp.Verified {
			t.Error("expected verification to fail")
		}
		if verifyResp.Status != types.DomainStatusFailedVerification {
			t.Errorf("expected failed_verification, got %s", verifyResp.Status)
		}
	})

	t.Run("Dev Localhost Bypass", func(t *testing.T) {
		addResp, _ := svc.AddCustomDomain(ctx, testSvc.ID, CreateDomainRequest{
			Domain: "my-app.localhost",
		})

		verifyResp, err := svc.VerifyCustomDomain(ctx, addResp.Domain.ID)
		if err != nil {
			t.Fatalf("VerifyCustomDomain failed: %v", err)
		}
		if !verifyResp.Verified {
			t.Error("expected localhost domain to be automatically verified in dev")
		}
	})
}

func TestSyncServiceRoutesForDeployment(t *testing.T) {
	ctx := context.Background()
	svc, repo, ingressDriver, _, _ := setupTestService()

	testSvc := &types.Service{
		ID:   "svc_sync_1",
		Name: "Web Dashboard",
		Slug: "web-dashboard",
		Port: 8080,
	}
	repo.services[testSvc.ID] = testSvc

	// Seed verified custom domain
	repo.domains["dom_1"] = &types.CustomDomain{
		ID:        "dom_1",
		ServiceID: testSvc.ID,
		Domain:    "dashboard.mycompany.org",
		Status:    types.DomainStatusActive,
	}

	result, err := svc.SyncServiceRoutesForDeployment(ctx, testSvc.ID, "http://alloc-node-3:24810")
	if err != nil {
		t.Fatalf("SyncServiceRoutesForDeployment failed: %v", err)
	}

	if len(result.SyncedHosts) != 2 {
		t.Errorf("expected 2 synced hosts, got %d: %+v", len(result.SyncedHosts), result.SyncedHosts)
	}

	// Verify platform host: web-dashboard.adroit.run
	platformRouteID := ingress.GenerateRouteID("web-dashboard.adroit.run")
	pr, err := ingressDriver.GetRoute(ctx, platformRouteID)
	if err != nil {
		t.Fatalf("platform route missing: %v", err)
	}
	if pr.Upstreams[0].Dial != "alloc-node-3:24810" {
		t.Errorf("expected upstream dial alloc-node-3:24810, got %s", pr.Upstreams[0].Dial)
	}

	// Verify custom domain route: dashboard.mycompany.org
	customRouteID := ingress.GenerateRouteID("dashboard.mycompany.org")
	cr, err := ingressDriver.GetRoute(ctx, customRouteID)
	if err != nil {
		t.Fatalf("custom domain route missing: %v", err)
	}
	if cr.Upstreams[0].Dial != "alloc-node-3:24810" {
		t.Errorf("expected upstream dial alloc-node-3:24810, got %s", cr.Upstreams[0].Dial)
	}
}

func TestDeleteCustomDomain(t *testing.T) {
	ctx := context.Background()
	svc, repo, ingressDriver, _, _ := setupTestService()

	testSvc := &types.Service{
		ID:   "svc_del",
		Name: "Delete Svc",
		Slug: "delete-svc",
	}
	repo.services[testSvc.ID] = testSvc

	addResp, _ := svc.AddCustomDomain(ctx, testSvc.ID, CreateDomainRequest{
		Domain: "todelete.org",
	})

	// Pre-create route
	routeID := ingress.GenerateRouteID("todelete.org")
	_ = ingressDriver.UpsertRoute(ctx, ingress.RouteSpec{
		ID:          routeID,
		HostPattern: "todelete.org",
		Upstreams:   []ingress.UpstreamTarget{{Dial: "127.0.0.1:80"}},
	})

	if err := svc.DeleteCustomDomain(ctx, addResp.Domain.ID); err != nil {
		t.Fatalf("DeleteCustomDomain failed: %v", err)
	}

	// Should be deleted from DB
	_, err := repo.GetDomainByID(ctx, addResp.Domain.ID)
	if !errors.Is(err, ErrDomainNotFound) {
		t.Errorf("expected ErrDomainNotFound in repo, got %v", err)
	}

	// Should be deleted from Ingress driver
	_, err = ingressDriver.GetRoute(ctx, routeID)
	if !errors.Is(err, ingress.ErrRouteNotFound) {
		t.Errorf("expected ErrRouteNotFound in driver, got %v", err)
	}
}
