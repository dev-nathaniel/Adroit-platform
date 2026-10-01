package projectservice

import (
	"context"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func setupTestService() (*Service, *MockRepository, *events.InMemBus) {
	repo := NewMockRepository()
	bus := events.NewInMemBus()
	svc := NewService(repo, WithPublisher(bus))
	return svc, repo, bus
}

func TestProjectService_Projects(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupTestService()

	userID := "usr_123"
	orgID := "org_abc"
	repo.AddMember(orgID, userID, "owner")

	t.Run("create project successfully", func(t *testing.T) {
		p, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
			OrganizationID: orgID,
			Name:           "Core Infrastructure",
			Description:    "Main backend systems",
		})
		if err != nil {
			t.Fatalf("unexpected error creating project: %v", err)
		}
		if p.ID == "" {
			t.Errorf("expected non-empty project ID")
		}
		if p.Slug != "core-infrastructure" {
			t.Errorf("expected slug 'core-infrastructure', got '%s'", p.Slug)
		}
	})

	t.Run("create project forbidden if user not in org", func(t *testing.T) {
		_, err := svc.CreateProject(ctx, "usr_stranger", CreateProjectRequest{
			OrganizationID: orgID,
			Name:           "Unauthorized Project",
		})
		if err == nil {
			t.Fatalf("expected forbidden error, got nil")
		}
	})

	t.Run("get project with service count", func(t *testing.T) {
		p, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
			OrganizationID: orgID,
			Name:           "Web Systems",
		})
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}

		detail, err := svc.GetProject(ctx, userID, p.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.ServiceCount != 0 {
			t.Errorf("expected 0 services, got %d", detail.ServiceCount)
		}
	})
}

func TestProjectService_StandaloneAndProjectServices(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupTestService()

	userID := "usr_123"
	orgID := "org_abc"
	repo.AddMember(orgID, userID, "owner")

	// Create a project
	proj, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
		OrganizationID: orgID,
		Name:           "Production Cluster",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	t.Run("create standalone service without project", func(t *testing.T) {
		s, err := svc.CreateService(ctx, userID, CreateServiceRequest{
			OrganizationID: orgID,
			ProjectID:      nil, // explicitly standalone!
			Name:           "Auth Worker",
			RepoURL:        "https://github.com/example/auth-worker",
		})
		if err != nil {
			t.Fatalf("unexpected error creating standalone service: %v", err)
		}
		if s.ProjectID != nil {
			t.Errorf("expected nil ProjectID for standalone service, got %v", *s.ProjectID)
		}
		if s.OrganizationID != orgID {
			t.Errorf("expected organization %s, got %s", orgID, s.OrganizationID)
		}
		if s.Port != 8080 {
			t.Errorf("expected default port 8080, got %d", s.Port)
		}
		if s.Type != types.ServiceTypeWeb {
			t.Errorf("expected default service type web, got %s", s.Type)
		}
	})

	t.Run("create service inside project", func(t *testing.T) {
		s, err := svc.CreateService(ctx, userID, CreateServiceRequest{
			OrganizationID: orgID,
			ProjectID:      &proj.ID,
			Name:           "API Service",
			RepoURL:        "https://github.com/example/api",
		})
		if err != nil {
			t.Fatalf("unexpected error creating in-project service: %v", err)
		}
		if s.ProjectID == nil || *s.ProjectID != proj.ID {
			t.Errorf("expected ProjectID %s, got %v", proj.ID, s.ProjectID)
		}
	})

	t.Run("move standalone service into project", func(t *testing.T) {
		// 1. Create standalone service
		standalone, err := svc.CreateService(ctx, userID, CreateServiceRequest{
			OrganizationID: orgID,
			Name:           "Standalone Redis Queue",
			RepoURL:        "https://github.com/example/queue",
		})
		if err != nil {
			t.Fatalf("failed to create standalone service: %v", err)
		}
		if standalone.ProjectID != nil {
			t.Fatalf("expected initial nil ProjectID")
		}

		// 2. Move into project
		moved, err := svc.MoveService(ctx, userID, standalone.ID, &proj.ID)
		if err != nil {
			t.Fatalf("unexpected error moving service into project: %v", err)
		}
		if moved.ProjectID == nil || *moved.ProjectID != proj.ID {
			t.Errorf("expected ProjectID %s after move, got %v", proj.ID, moved.ProjectID)
		}

		// 3. Move back out to standalone
		standaloneAgain, err := svc.MoveService(ctx, userID, standalone.ID, nil)
		if err != nil {
			t.Fatalf("unexpected error detaching service to standalone: %v", err)
		}
		if standaloneAgain.ProjectID != nil {
			t.Errorf("expected nil ProjectID after moving to standalone, got %v", *standaloneAgain.ProjectID)
		}
	})

	t.Run("cross-tenant security: cannot move service to project in another organization", func(t *testing.T) {
		// Org 2 & Project 2
		org2ID := "org_tenant_two"
		repo.AddMember(org2ID, userID, "owner")
		proj2, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
			OrganizationID: org2ID,
			Name:           "Tenant Two Core",
		})
		if err != nil {
			t.Fatalf("failed to create tenant 2 project: %v", err)
		}

		// Service in Org 1
		s1, err := svc.CreateService(ctx, userID, CreateServiceRequest{
			OrganizationID: orgID,
			Name:           "Tenant 1 Workload",
			RepoURL:        "https://github.com/example/tenant1",
		})
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		// Attempt to move Org 1 service into Org 2's project
		_, err = svc.MoveService(ctx, userID, s1.ID, &proj2.ID)
		if err == nil {
			t.Fatalf("expected error moving service across organization boundaries, got nil")
		}
	})

	t.Run("deleting project detaches services to standalone", func(t *testing.T) {
		tempProj, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
			OrganizationID: orgID,
			Name:           "Temporary Project",
		})
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}

		svcInProj, err := svc.CreateService(ctx, userID, CreateServiceRequest{
			OrganizationID: orgID,
			ProjectID:      &tempProj.ID,
			Name:           "Orphan Candidate",
			RepoURL:        "https://github.com/example/orphan",
		})
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		// Delete project
		if err := svc.DeleteProject(ctx, userID, tempProj.ID); err != nil {
			t.Fatalf("failed to delete project: %v", err)
		}

		// Verify service still exists but is now standalone
		refreshed, err := svc.GetService(ctx, userID, svcInProj.ID)
		if err != nil {
			t.Fatalf("service should still exist after project deletion: %v", err)
		}
		if refreshed.ProjectID != nil {
			t.Errorf("expected ProjectID to be nil after project deletion, got %v", *refreshed.ProjectID)
		}
	})
}

func TestProjectService_EnvironmentVariables(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := setupTestService()

	userID := "usr_123"
	orgID := "org_abc"
	repo.AddMember(orgID, userID, "owner")

	proj, err := svc.CreateProject(ctx, userID, CreateProjectRequest{
		OrganizationID: orgID,
		Name:           "Env Project",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	service, err := svc.CreateService(ctx, userID, CreateServiceRequest{
		OrganizationID: orgID,
		ProjectID:      &proj.ID,
		Name:           "Env App",
		RepoURL:        "https://github.com/example/app",
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("project and service env vars resolution", func(t *testing.T) {
		// Set project env vars
		err := svc.SetProjectEnvVars(ctx, userID, proj.ID, []EnvVarItem{
			{Key: "PORT", Value: "3000", IsSecret: false},
			{Key: "NODE_ENV", Value: "production", IsSecret: false},
		})
		if err != nil {
			t.Fatalf("failed to set project env vars: %v", err)
		}

		// Set service env vars (PORT should override project PORT)
		err = svc.SetServiceEnvVars(ctx, userID, service.ID, []EnvVarItem{
			{Key: "PORT", Value: "8080", IsSecret: false},
			{Key: "DATABASE_URL", Value: "postgres://localhost:5432/mydb", IsSecret: true},
		})
		if err != nil {
			t.Fatalf("failed to set service env vars: %v", err)
		}

		// Resolve merged env vars
		resolved, err := svc.GetResolvedServiceEnv(ctx, userID, service.ID)
		if err != nil {
			t.Fatalf("failed to get resolved env: %v", err)
		}

		if resolved["PORT"] != "8080" {
			t.Errorf("expected service PORT=8080 to override project PORT, got '%s'", resolved["PORT"])
		}
		if resolved["NODE_ENV"] != "production" {
			t.Errorf("expected inherited NODE_ENV=production, got '%s'", resolved["NODE_ENV"])
		}
		if resolved["DATABASE_URL"] != "postgres://localhost:5432/mydb" {
			t.Errorf("expected service DATABASE_URL, got '%s'", resolved["DATABASE_URL"])
		}

		// Verify API GET masks secrets
		apiVars, err := svc.GetServiceEnvVars(ctx, userID, service.ID)
		if err != nil {
			t.Fatalf("failed to get service env vars: %v", err)
		}
		for _, ev := range apiVars {
			if ev.Key == "DATABASE_URL" {
				if ev.Value != "••••••••" {
					t.Errorf("expected masked secret value '••••••••', got '%s'", ev.Value)
				}
				if !ev.IsSecret {
					t.Errorf("expected IsSecret to be true")
				}
			}
		}

		// Verify Reveal endpoint retrieves raw secret from Vault
		revealed, err := svc.RevealServiceSecret(ctx, userID, service.ID, "DATABASE_URL")
		if err != nil {
			t.Fatalf("failed to reveal secret: %v", err)
		}
		if revealed.Value != "postgres://localhost:5432/mydb" {
			t.Errorf("expected revealed secret 'postgres://localhost:5432/mydb', got '%s'", revealed.Value)
		}
		if revealed.VaultPath == "" {
			t.Errorf("expected non-empty VaultPath in revealed secret response")
		}
	})
}

