package projectservice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dev-nathaniel/Adroit-platform/pkg/events"
	"github.com/dev-nathaniel/Adroit-platform/pkg/storage"
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
	"github.com/dev-nathaniel/Adroit-platform/pkg/vault"
)

var (
	nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9]+`)
	validEnvKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Service Option pattern
type Option func(*Service)

// WithPublisher sets the event publisher for the service
func WithPublisher(pub events.Publisher) Option {
	return func(s *Service) {
		s.publisher = pub
	}
}

// WithSecretStore sets the Vault secret store for the service
func WithSecretStore(store vault.SecretStore) Option {
	return func(s *Service) {
		s.secretStore = store
	}
}

// WithStorageProvider sets the artifact/source storage provider for the service
func WithStorageProvider(store storage.StorageProvider) Option {
	return func(s *Service) {
		s.storage = store
	}
}

// Service contains core business logic for projects, services, and environment variables
type Service struct {
	repo        Repository
	publisher   events.Publisher
	secretStore vault.SecretStore
	storage     storage.StorageProvider
}

// NewService creates a new project-service business engine
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:        repo,
		publisher:   events.NewNoopPublisher(),
		secretStore: vault.NewSimulatedSecretStore(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.storage == nil {
		s.storage = storage.NewStorageProviderFromEnv("data/uploads/sources", "/api/v1/services")
	}
	return s
}

// ============================================================================
// PROJECTS
// ============================================================================

// CreateProject creates a new project in an organization
func (s *Service) CreateProject(ctx context.Context, userID string, req CreateProjectRequest) (*types.Project, error) {
	if strings.TrimSpace(req.OrganizationID) == "" {
		return nil, fmt.Errorf("%w: organization_id is required", ErrInvalidRequest)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: project name is required", ErrInvalidRequest)
	}

	// Verify caller belongs to organization
	_, err := s.repo.CheckOrgMembership(ctx, req.OrganizationID, userID)
	if err != nil {
		return nil, err
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	} else {
		slug = generateSlug(slug)
	}

	// Ensure slug uniqueness within organization
	if existing, _ := s.repo.GetProjectBySlug(ctx, req.OrganizationID, slug); existing != nil {
		slug = fmt.Sprintf("%s-%s", slug, uuid.New().String()[:6])
	}

	now := time.Now().UTC()
	project := &types.Project{
		ID:             uuid.New().String(),
		OrganizationID: req.OrganizationID,
		Name:           name,
		Slug:           slug,
		Description:    strings.TrimSpace(req.Description),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.CreateProject(ctx, project); err != nil {
		return nil, err
	}

	// Publish domain event
	_ = s.publisher.Publish(ctx, events.SubjectProjectCreated, events.New(
		events.SubjectProjectCreated,
		"project-service",
		events.ProjectCreatedPayload{
			ProjectID:      project.ID,
			OrganizationID: project.OrganizationID,
			Name:           project.Name,
			Slug:           project.Slug,
		},
	))

	return project, nil
}

// GetProject returns a project with service count
func (s *Service) GetProject(ctx context.Context, userID, projectID string) (*ProjectDetail, error) {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Check org membership
	if _, err := s.repo.CheckOrgMembership(ctx, project.OrganizationID, userID); err != nil {
		return nil, err
	}

	count, _ := s.repo.CountServicesInProject(ctx, projectID)
	services, _ := s.repo.ListServices(ctx, project.OrganizationID, &projectID, false)

	return &ProjectDetail{
		Project:      *project,
		ServiceCount: count,
		Services:     services,
	}, nil
}

// ListProjects lists projects by organization, or across all orgs the user belongs to
func (s *Service) ListProjects(ctx context.Context, userID, orgID string) ([]types.Project, error) {
	if orgID != "" {
		if _, err := s.repo.CheckOrgMembership(ctx, orgID, userID); err != nil {
			return nil, err
		}
		return s.repo.ListProjectsByOrg(ctx, orgID)
	}
	return s.repo.ListProjectsByUser(ctx, userID)
}

// UpdateProject updates mutable project details
func (s *Service) UpdateProject(ctx context.Context, userID, projectID string, req UpdateProjectRequest) (*types.Project, error) {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, project.OrganizationID, userID); err != nil {
		return nil, err
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		project.Name = strings.TrimSpace(*req.Name)
	}
	if req.Slug != nil && strings.TrimSpace(*req.Slug) != "" {
		newSlug := generateSlug(*req.Slug)
		if newSlug != project.Slug {
			if existing, _ := s.repo.GetProjectBySlug(ctx, project.OrganizationID, newSlug); existing != nil && existing.ID != project.ID {
				return nil, fmt.Errorf("%w: project slug already in use", ErrConflict)
			}
			project.Slug = newSlug
		}
	}
	if req.Description != nil {
		project.Description = strings.TrimSpace(*req.Description)
	}
	project.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateProject(ctx, project); err != nil {
		return nil, err
	}

	_ = s.publisher.Publish(ctx, events.SubjectProjectUpdated, events.New(
		events.SubjectProjectUpdated,
		"project-service",
		events.ProjectUpdatedPayload{
			ProjectID:      project.ID,
			OrganizationID: project.OrganizationID,
			Name:           project.Name,
			Slug:           project.Slug,
		},
	))

	return project, nil
}

// DeleteProject deletes a project (attached services become standalone)
func (s *Service) DeleteProject(ctx context.Context, userID, projectID string) error {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return err
	}

	role, err := s.repo.CheckOrgMembership(ctx, project.OrganizationID, userID)
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return fmt.Errorf("%w: only owner or admin can delete projects", ErrForbidden)
	}

	if err := s.repo.DeleteProject(ctx, projectID); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectProjectDeleted, events.New(
		events.SubjectProjectDeleted,
		"project-service",
		events.ProjectDeletedPayload{
			ProjectID:      project.ID,
			OrganizationID: project.OrganizationID,
		},
	))

	return nil
}

// ============================================================================
// SERVICES (STANDALONE & IN-PROJECT)
// ============================================================================

// CreateService creates a new service (standalone if project_id is nil, or within a project)
func (s *Service) CreateService(ctx context.Context, userID string, req CreateServiceRequest) (*types.Service, error) {
	if strings.TrimSpace(req.OrganizationID) == "" {
		return nil, fmt.Errorf("%w: organization_id is required", ErrInvalidRequest)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: service name is required", ErrInvalidRequest)
	}

	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = types.SourceTypeGit
	}

	repoURL := strings.TrimSpace(req.RepoURL)
	archiveURL := strings.TrimSpace(req.ArchiveURL)
	imageURL := strings.TrimSpace(req.ImageURL)

	switch sourceType {
	case types.SourceTypeGit:
		if repoURL == "" {
			return nil, fmt.Errorf("%w: repo_url is required for git source type", ErrInvalidRequest)
		}
	case types.SourceTypeImage:
		if imageURL == "" && repoURL != "" {
			imageURL = repoURL
		}
		if imageURL == "" {
			return nil, fmt.Errorf("%w: image_url is required for image source type", ErrInvalidRequest)
		}
	case types.SourceTypeArchive:
		if archiveURL == "" && repoURL != "" && (strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://")) {
			archiveURL = repoURL
		}
		// Note: archive_url can be initially empty, uploaded subsequently via /upload endpoint
	default:
		return nil, fmt.Errorf("%w: invalid source_type '%s', allowed: git, archive, image", ErrInvalidRequest, sourceType)
	}

	// Verify org membership
	if _, err := s.repo.CheckOrgMembership(ctx, req.OrganizationID, userID); err != nil {
		return nil, err
	}

	var targetProjectID *string
	if req.ProjectID != nil && strings.TrimSpace(*req.ProjectID) != "" {
		pid := strings.TrimSpace(*req.ProjectID)
		proj, err := s.repo.GetProjectByID(ctx, pid)
		if err != nil {
			return nil, fmt.Errorf("target project not found: %w", err)
		}
		if proj.OrganizationID != req.OrganizationID {
			return nil, fmt.Errorf("%w: project belongs to a different organization", ErrForbidden)
		}
		targetProjectID = &pid
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	} else {
		slug = generateSlug(slug)
	}

	// Check slug uniqueness within organization
	if existing, _ := s.repo.GetServiceBySlug(ctx, req.OrganizationID, slug); existing != nil {
		slug = fmt.Sprintf("%s-%s", slug, uuid.New().String()[:6])
	}

	sType := req.Type
	if sType == "" {
		sType = types.ServiceTypeWeb
	}
	rTier := req.RuntimeTier
	if rTier == "" {
		rTier = types.RuntimeTierGeneral
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	port := req.Port
	if port <= 0 {
		port = 8080
	}
	minRep := req.MinReplicas
	if minRep <= 0 {
		minRep = 1
	}
	maxRep := req.MaxReplicas
	if maxRep <= 0 {
		maxRep = 1
	}
	cpu := req.CPUCores
	if cpu <= 0 {
		cpu = 0.5
	}
	mem := req.MemoryMB
	if mem <= 0 {
		mem = 512
	}
	disk := req.DiskMB
	if disk <= 0 {
		disk = 2048
	}
	healthPath := strings.TrimSpace(req.HealthCheckPath)
	if healthPath == "" {
		healthPath = "/healthz"
	}

	now := time.Now().UTC()
	svc := &types.Service{
		ID:             uuid.New().String(),
		OrganizationID: req.OrganizationID,
		ProjectID:      targetProjectID,
		Name:           name,
		Slug:           slug,
		Type:           sType,
		RuntimeTier:    rTier,
		SourceType:     sourceType,
		RepoURL:        repoURL,
		ArchiveURL:     archiveURL,
		ImageURL:       imageURL,
		Branch:         branch,
		RootDirectory:  strings.TrimSpace(req.RootDirectory),
		BuildCommand:   strings.TrimSpace(req.BuildCommand),
		StartCommand:   strings.TrimSpace(req.StartCommand),
		DockerfilePath: strings.TrimSpace(req.DockerfilePath),
		Port:           port,
		MinReplicas:    minRep,
		MaxReplicas:    maxRep,
		Resources: types.ResourceLimits{
			CPUCores: cpu,
			MemoryMB: mem,
			DiskMB:   disk,
		},
		HealthCheck: types.HealthCheckConfig{
			Path: healthPath,
			Port: port,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateService(ctx, svc); err != nil {
		return nil, err
	}

	_ = s.publisher.Publish(ctx, events.SubjectServiceCreated, events.New(
		events.SubjectServiceCreated,
		"project-service",
		events.ServiceCreatedPayload{
			ServiceID:      svc.ID,
			OrganizationID: svc.OrganizationID,
			ProjectID:      svc.ProjectID,
			Name:           svc.Name,
			Slug:           svc.Slug,
			Type:           string(svc.Type),
			SourceType:     string(svc.SourceType),
			RepoURL:        svc.RepoURL,
			ArchiveURL:     svc.ArchiveURL,
			ImageURL:       svc.ImageURL,
			Branch:         svc.Branch,
		},
	))

	return svc, nil
}

// GetService retrieves a service by ID
func (s *Service) GetService(ctx context.Context, userID, serviceID string) (*types.Service, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	return svc, nil
}

// ListServices lists services within an organization (optionally filtered by project or standalone)
func (s *Service) ListServices(ctx context.Context, userID, orgID string, projectID *string, standaloneOnly bool) ([]types.Service, error) {
	if strings.TrimSpace(orgID) == "" {
		return nil, fmt.Errorf("%w: organization_id is required", ErrInvalidRequest)
	}

	if _, err := s.repo.CheckOrgMembership(ctx, orgID, userID); err != nil {
		return nil, err
	}

	if projectID != nil && *projectID != "" {
		proj, err := s.repo.GetProjectByID(ctx, *projectID)
		if err != nil {
			return nil, err
		}
		if proj.OrganizationID != orgID {
			return nil, fmt.Errorf("%w: project does not belong to specified organization", ErrForbidden)
		}
	}

	return s.repo.ListServices(ctx, orgID, projectID, standaloneOnly)
}

// UpdateService modifies mutable fields of a service
func (s *Service) UpdateService(ctx context.Context, userID, serviceID string, req UpdateServiceRequest) (*types.Service, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		svc.Name = strings.TrimSpace(*req.Name)
	}
	if req.Slug != nil && strings.TrimSpace(*req.Slug) != "" {
		newSlug := generateSlug(*req.Slug)
		if newSlug != svc.Slug {
			if existing, _ := s.repo.GetServiceBySlug(ctx, svc.OrganizationID, newSlug); existing != nil && existing.ID != svc.ID {
				return nil, fmt.Errorf("%w: service slug already in use", ErrConflict)
			}
			svc.Slug = newSlug
		}
	}
	if req.Type != nil {
		svc.Type = *req.Type
	}
	if req.RuntimeTier != nil {
		svc.RuntimeTier = *req.RuntimeTier
	}
	if req.SourceType != nil {
		svc.SourceType = *req.SourceType
	}
	if req.RepoURL != nil {
		svc.RepoURL = strings.TrimSpace(*req.RepoURL)
	}
	if req.ArchiveURL != nil {
		svc.ArchiveURL = strings.TrimSpace(*req.ArchiveURL)
	}
	if req.ImageURL != nil {
		svc.ImageURL = strings.TrimSpace(*req.ImageURL)
	}
	if req.Branch != nil && strings.TrimSpace(*req.Branch) != "" {
		svc.Branch = strings.TrimSpace(*req.Branch)
	}
	if req.RootDirectory != nil {
		svc.RootDirectory = strings.TrimSpace(*req.RootDirectory)
	}
	if req.BuildCommand != nil {
		svc.BuildCommand = strings.TrimSpace(*req.BuildCommand)
	}
	if req.StartCommand != nil {
		svc.StartCommand = strings.TrimSpace(*req.StartCommand)
	}
	if req.DockerfilePath != nil {
		svc.DockerfilePath = strings.TrimSpace(*req.DockerfilePath)
	}
	if req.Port != nil && *req.Port > 0 {
		svc.Port = *req.Port
		svc.HealthCheck.Port = *req.Port
	}
	if req.MinReplicas != nil && *req.MinReplicas > 0 {
		svc.MinReplicas = *req.MinReplicas
	}
	if req.MaxReplicas != nil && *req.MaxReplicas > 0 {
		svc.MaxReplicas = *req.MaxReplicas
	}
	if req.CPUCores != nil && *req.CPUCores > 0 {
		svc.Resources.CPUCores = *req.CPUCores
	}
	if req.MemoryMB != nil && *req.MemoryMB > 0 {
		svc.Resources.MemoryMB = *req.MemoryMB
	}
	if req.DiskMB != nil && *req.DiskMB > 0 {
		svc.Resources.DiskMB = *req.DiskMB
	}
	if req.HealthCheckPath != nil && strings.TrimSpace(*req.HealthCheckPath) != "" {
		svc.HealthCheck.Path = strings.TrimSpace(*req.HealthCheckPath)
	}

	svc.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateService(ctx, svc); err != nil {
		return nil, err
	}

	_ = s.publisher.Publish(ctx, events.SubjectServiceUpdated, events.New(
		events.SubjectServiceUpdated,
		"project-service",
		events.ServiceUpdatedPayload{
			ServiceID:      svc.ID,
			OrganizationID: svc.OrganizationID,
			ProjectID:      svc.ProjectID,
			Name:           svc.Name,
			Slug:           svc.Slug,
		},
	))

	return svc, nil
}

// UploadServiceSource persists a source code archive (.zip, .tar.gz, .tar),
// updates the service record with the archive URL, and optionally triggers a build.
func (s *Service) UploadServiceSource(ctx context.Context, userID, serviceID string, filename string, r io.Reader, triggerBuild bool) (*UploadSourceResponse, error) {
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidRequest)
	}

	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	normalized, err := storage.NormalizeToTarGz(r, filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	key := fmt.Sprintf("%s/%s", svc.ID, normalized.Filename)
	archiveURL, err := s.storage.Save(ctx, key, bytes.NewReader(normalized.TarGzData))
	if err != nil {
		return nil, fmt.Errorf("failed to save archive to storage: %w", err)
	}

	// Update service with archive source
	svc.SourceType = types.SourceTypeArchive
	svc.ArchiveURL = archiveURL
	svc.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateService(ctx, svc); err != nil {
		return nil, fmt.Errorf("failed to update service source: %w", err)
	}

	var buildID string
	if triggerBuild {
		buildID = uuid.New().String()
		shortHash := normalized.Checksum
		if len(shortHash) > 19 {
			shortHash = shortHash[7:19]
		}
		build := &types.Build{
			ID:          buildID,
			ServiceID:   svc.ID,
			CommitHash:  shortHash,
			Branch:      "archive",
			Message:     fmt.Sprintf("Archive upload deployment: %s", normalized.Filename),
			Author:      "user:" + userID,
			Status:      types.BuildStatusQueued,
			SourceType:  types.SourceTypeArchive,
			ArchiveURL:  archiveURL,
			TriggeredBy: "upload:" + userID,
			CreatedAt:   time.Now().UTC(),
		}
		if bErr := s.repo.CreateBuild(ctx, build); bErr == nil {
			projID := ""
			if svc.ProjectID != nil {
				projID = *svc.ProjectID
			}
			_ = s.publisher.Publish(ctx, events.SubjectBuildRequested, events.New(
				events.SubjectBuildRequested,
				"project-service",
				events.BuildRequestedPayload{
					BuildID:        build.ID,
					ServiceID:      svc.ID,
					ProjectID:      projID,
					SourceType:     string(types.SourceTypeArchive),
					ArchiveURL:     archiveURL,
					Branch:         "archive",
					CommitHash:     shortHash,
					DockerfilePath: svc.DockerfilePath,
				},
			))
		}
	}

	return &UploadSourceResponse{
		ServiceID:  svc.ID,
		SourceType: types.SourceTypeArchive,
		ArchiveURL: archiveURL,
		SizeBytes:  normalized.SizeBytes,
		Checksum:   normalized.Checksum,
		BuildID:    buildID,
		Message:    "Source code bundle uploaded successfully",
	}, nil
}

// DownloadServiceSource streams the active archive bundle for a service
func (s *Service) DownloadServiceSource(ctx context.Context, userID, serviceID string) (io.ReadCloser, int64, string, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, 0, "", err
	}

	if userID != "" {
		if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
			return nil, 0, "", err
		}
	}

	if svc.ArchiveURL == "" {
		return nil, 0, "", fmt.Errorf("%w: service does not have an uploaded archive", ErrNotFound)
	}

	filename := filepath.Base(svc.ArchiveURL)
	key := fmt.Sprintf("%s/%s", svc.ID, filename)
	rc, size, err := s.storage.Open(ctx, key)
	if err != nil {
		// Try opening directly by filename
		rc2, size2, err2 := s.storage.Open(ctx, filename)
		if err2 != nil {
			return nil, 0, "", fmt.Errorf("%w: failed to open source archive: %v", ErrNotFound, err)
		}
		return rc2, size2, filename, nil
	}

	return rc, size, filename, nil
}

// MoveService moves a service to a different project or detaches it into a standalone service
func (s *Service) MoveService(ctx context.Context, userID, serviceID string, targetProjectID *string) (*types.Service, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	oldProjectID := svc.ProjectID
	var newProjectID *string

	if targetProjectID != nil && strings.TrimSpace(*targetProjectID) != "" {
		cleanTarget := strings.TrimSpace(*targetProjectID)
		targetProj, err := s.repo.GetProjectByID(ctx, cleanTarget)
		if err != nil {
			return nil, fmt.Errorf("target project not found: %w", err)
		}

		// Security: Must belong to the exact same organization
		if targetProj.OrganizationID != svc.OrganizationID {
			return nil, fmt.Errorf("%w: target project belongs to a different organization", ErrForbidden)
		}
		newProjectID = &cleanTarget
	}

	if err := s.repo.MoveService(ctx, serviceID, newProjectID); err != nil {
		return nil, err
	}

	// Refetch refreshed service
	refreshed, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	_ = s.publisher.Publish(ctx, events.SubjectServiceMoved, events.New(
		events.SubjectServiceMoved,
		"project-service",
		events.ServiceMovedPayload{
			ServiceID:      svc.ID,
			OrganizationID: svc.OrganizationID,
			OldProjectID:   oldProjectID,
			NewProjectID:   newProjectID,
		},
	))

	return refreshed, nil
}

// DeleteService removes a service by ID
func (s *Service) DeleteService(ctx context.Context, userID, serviceID string) error {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}

	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return err
	}

	if err := s.repo.DeleteService(ctx, serviceID); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectServiceDeleted, events.New(
		events.SubjectServiceDeleted,
		"project-service",
		events.ServiceDeletedPayload{
			ServiceID:      svc.ID,
			OrganizationID: svc.OrganizationID,
			ProjectID:      svc.ProjectID,
		},
	))

	return nil
}

// ============================================================================
// ENVIRONMENT VARIABLES
// ============================================================================

// SetServiceEnvVars upserts service environment variables and stores secrets in Vault
func (s *Service) SetServiceEnvVars(ctx context.Context, userID, serviceID string, vars []EnvVarItem) error {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return err
	}

	secretsToStore := make(map[string]string)
	var keysUpdated []string
	cleanVars := make([]EnvVarItem, len(vars))
	copy(cleanVars, vars)

	for i := range cleanVars {
		cleanKey := strings.TrimSpace(cleanVars[i].Key)
		if !validEnvKeyRegex.MatchString(cleanKey) {
			return fmt.Errorf("%w: invalid environment variable key '%s'", ErrInvalidRequest, cleanKey)
		}
		cleanVars[i].Key = cleanKey
		keysUpdated = append(keysUpdated, cleanKey)

		if cleanVars[i].Scope == "" {
			cleanVars[i].Scope = types.EnvVarScopeAll
		}
		if cleanVars[i].TargetEnv == "" {
			cleanVars[i].TargetEnv = types.EnvVarTargetEnvAll
		}

		if cleanVars[i].IsSecret {
			secretsToStore[cleanKey] = cleanVars[i].Value
			cleanVars[i].VaultPath = s.secretStore.VaultPath(svc.OrganizationID, vault.ScopeService, serviceID)
			cleanVars[i].Value = "[vault:secret]"
		}
	}

	// 1. Store secrets securely in Vault
	if len(secretsToStore) > 0 {
		if err := s.secretStore.PutSecrets(ctx, svc.OrganizationID, vault.ScopeService, serviceID, secretsToStore); err != nil {
			return fmt.Errorf("failed to store secrets in vault: %w", err)
		}
	}

	// 2. Persist metadata & non-secret values in PostgreSQL
	if err := s.repo.SetServiceEnvVars(ctx, svc.OrganizationID, serviceID, cleanVars); err != nil {
		return err
	}

	// 3. Publish domain event
	_ = s.publisher.Publish(ctx, events.SubjectEnvVarsUpdated, events.New(
		events.SubjectEnvVarsUpdated,
		"project-service",
		events.EnvVarsUpdatedPayload{
			OrganizationID: svc.OrganizationID,
			ProjectID:      svc.ProjectID,
			ServiceID:      &serviceID,
			UpdatedKeys:    keysUpdated,
			Action:         "set",
		},
	))

	return nil
}

// GetServiceEnvVars returns service environment variables with secrets masked
func (s *Service) GetServiceEnvVars(ctx context.Context, userID, serviceID string) ([]types.EnvironmentVariable, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	vars, err := s.repo.GetServiceEnvVars(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	// Mask secret values in API response
	for i := range vars {
		if vars[i].IsSecret {
			vars[i].Value = "••••••••"
		}
	}

	return vars, nil
}

// RevealServiceSecret returns the unmasked value of a secret stored in Vault for authorized members
func (s *Service) RevealServiceSecret(ctx context.Context, userID, serviceID, key string) (*RevealSecretResponse, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	ev, err := s.repo.GetEnvVarByKey(ctx, &serviceID, nil, key)
	if err != nil {
		return nil, err
	}

	if !ev.IsSecret {
		return &RevealSecretResponse{
			Key:       ev.Key,
			Value:     ev.Value,
			VaultPath: "",
		}, nil
	}

	rawVal, err := s.secretStore.GetSecretKey(ctx, svc.OrganizationID, vault.ScopeService, serviceID, key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret from vault: %w", err)
	}

	return &RevealSecretResponse{
		Key:       key,
		Value:     rawVal,
		VaultPath: ev.VaultPath,
	}, nil
}

// DeleteServiceEnvVar removes an environment variable from a service and Vault
func (s *Service) DeleteServiceEnvVar(ctx context.Context, userID, serviceID, key string) error {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return err
	}

	if err := s.repo.DeleteServiceEnvVar(ctx, serviceID, key); err != nil {
		return err
	}

	// Remove from Vault
	_ = s.secretStore.DeleteSecretKey(ctx, svc.OrganizationID, vault.ScopeService, serviceID, key)

	// Publish domain event
	_ = s.publisher.Publish(ctx, events.SubjectEnvVarsUpdated, events.New(
		events.SubjectEnvVarsUpdated,
		"project-service",
		events.EnvVarsUpdatedPayload{
			OrganizationID: svc.OrganizationID,
			ProjectID:      svc.ProjectID,
			ServiceID:      &serviceID,
			UpdatedKeys:    []string{key},
			Action:         "delete",
		},
	))

	return nil
}

// SetProjectEnvVars upserts project environment variables and stores secrets in Vault
func (s *Service) SetProjectEnvVars(ctx context.Context, userID, projectID string, vars []EnvVarItem) error {
	proj, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, proj.OrganizationID, userID); err != nil {
		return err
	}

	secretsToStore := make(map[string]string)
	var keysUpdated []string
	cleanVars := make([]EnvVarItem, len(vars))
	copy(cleanVars, vars)

	for i := range cleanVars {
		cleanKey := strings.TrimSpace(cleanVars[i].Key)
		if !validEnvKeyRegex.MatchString(cleanKey) {
			return fmt.Errorf("%w: invalid environment variable key '%s'", ErrInvalidRequest, cleanKey)
		}
		cleanVars[i].Key = cleanKey
		keysUpdated = append(keysUpdated, cleanKey)

		if cleanVars[i].Scope == "" {
			cleanVars[i].Scope = types.EnvVarScopeAll
		}
		if cleanVars[i].TargetEnv == "" {
			cleanVars[i].TargetEnv = types.EnvVarTargetEnvAll
		}

		if cleanVars[i].IsSecret {
			secretsToStore[cleanKey] = cleanVars[i].Value
			cleanVars[i].VaultPath = s.secretStore.VaultPath(proj.OrganizationID, vault.ScopeProject, projectID)
			cleanVars[i].Value = "[vault:secret]"
		}
	}

	if len(secretsToStore) > 0 {
		if err := s.secretStore.PutSecrets(ctx, proj.OrganizationID, vault.ScopeProject, projectID, secretsToStore); err != nil {
			return fmt.Errorf("failed to store secrets in vault: %w", err)
		}
	}

	if err := s.repo.SetProjectEnvVars(ctx, proj.OrganizationID, projectID, cleanVars); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectEnvVarsUpdated, events.New(
		events.SubjectEnvVarsUpdated,
		"project-service",
		events.EnvVarsUpdatedPayload{
			OrganizationID: proj.OrganizationID,
			ProjectID:      &projectID,
			ServiceID:      nil,
			UpdatedKeys:    keysUpdated,
			Action:         "set",
		},
	))

	return nil
}

// GetProjectEnvVars returns project environment variables with secrets masked
func (s *Service) GetProjectEnvVars(ctx context.Context, userID, projectID string) ([]types.EnvironmentVariable, error) {
	proj, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, proj.OrganizationID, userID); err != nil {
		return nil, err
	}

	vars, err := s.repo.GetProjectEnvVars(ctx, projectID)
	if err != nil {
		return nil, err
	}

	for i := range vars {
		if vars[i].IsSecret {
			vars[i].Value = "••••••••"
		}
	}

	return vars, nil
}

// RevealProjectSecret returns the unmasked value of a project secret from Vault
func (s *Service) RevealProjectSecret(ctx context.Context, userID, projectID, key string) (*RevealSecretResponse, error) {
	proj, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, proj.OrganizationID, userID); err != nil {
		return nil, err
	}

	ev, err := s.repo.GetEnvVarByKey(ctx, nil, &projectID, key)
	if err != nil {
		return nil, err
	}

	if !ev.IsSecret {
		return &RevealSecretResponse{
			Key:       ev.Key,
			Value:     ev.Value,
			VaultPath: "",
		}, nil
	}

	rawVal, err := s.secretStore.GetSecretKey(ctx, proj.OrganizationID, vault.ScopeProject, projectID, key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret from vault: %w", err)
	}

	return &RevealSecretResponse{
		Key:       key,
		Value:     rawVal,
		VaultPath: ev.VaultPath,
	}, nil
}

// DeleteProjectEnvVar removes an environment variable from a project and Vault
func (s *Service) DeleteProjectEnvVar(ctx context.Context, userID, projectID, key string) error {
	proj, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		return err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, proj.OrganizationID, userID); err != nil {
		return err
	}

	if err := s.repo.DeleteProjectEnvVar(ctx, projectID, key); err != nil {
		return err
	}

	_ = s.secretStore.DeleteSecretKey(ctx, proj.OrganizationID, vault.ScopeProject, projectID, key)

	_ = s.publisher.Publish(ctx, events.SubjectEnvVarsUpdated, events.New(
		events.SubjectEnvVarsUpdated,
		"project-service",
		events.EnvVarsUpdatedPayload{
			OrganizationID: proj.OrganizationID,
			ProjectID:      &projectID,
			ServiceID:      nil,
			UpdatedKeys:    []string{key},
			Action:         "delete",
		},
	))

	return nil
}

// GetResolvedServiceEnv returns unmasked runtime environment variables (project merged with service overriding, secrets read from Vault)
func (s *Service) GetResolvedServiceEnv(ctx context.Context, userID, serviceID string) (map[string]string, error) {
	svc, err := s.repo.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.CheckOrgMembership(ctx, svc.OrganizationID, userID); err != nil {
		return nil, err
	}

	result := make(map[string]string)

	// 1. Load project variables if in a project
	if svc.ProjectID != nil && *svc.ProjectID != "" {
		pVars, err := s.repo.GetProjectEnvVars(ctx, *svc.ProjectID)
		if err == nil {
			var projectVaultSecrets map[string]string
			for _, pv := range pVars {
				if pv.IsSecret {
					if projectVaultSecrets == nil {
						projectVaultSecrets, _ = s.secretStore.GetSecrets(ctx, svc.OrganizationID, vault.ScopeProject, *svc.ProjectID)
					}
					if secretVal, ok := projectVaultSecrets[pv.Key]; ok {
						result[pv.Key] = secretVal
					}
				} else {
					result[pv.Key] = pv.Value
				}
			}
		}
	}

	// 2. Load service variables (service overrides project)
	sVars, err := s.repo.GetServiceEnvVars(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	var serviceVaultSecrets map[string]string
	for _, sv := range sVars {
		if sv.IsSecret {
			if serviceVaultSecrets == nil {
				serviceVaultSecrets, _ = s.secretStore.GetSecrets(ctx, svc.OrganizationID, vault.ScopeService, serviceID)
			}
			if secretVal, ok := serviceVaultSecrets[sv.Key]; ok {
				result[sv.Key] = secretVal
			}
		} else {
			result[sv.Key] = sv.Value
		}
	}

	return result, nil
}

// generateSlug formats a human string into an alphanumeric hyphenated slug
func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphaNumRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "app"
	}
	return slug
}
