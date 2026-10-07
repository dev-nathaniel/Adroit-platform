package projectservice

import (
	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// CreateProjectRequest holds inputs for creating a project
type CreateProjectRequest struct {
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug,omitempty"`
	Description    string `json:"description,omitempty"`
}

// UpdateProjectRequest holds mutable project fields
type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

// ProjectDetail contains project info plus service count
type ProjectDetail struct {
	types.Project
	ServiceCount int             `json:"service_count"`
	Services     []types.Service `json:"services,omitempty"`
}

// CreateServiceRequest holds inputs for creating a service (standalone or inside a project)
type CreateServiceRequest struct {
	OrganizationID  string            `json:"organization_id"`
	ProjectID       *string           `json:"project_id,omitempty"` // nil/omitted -> standalone service
	Name            string            `json:"name"`
	Slug            string            `json:"slug,omitempty"`
	Type            types.ServiceType  `json:"type,omitempty"`         // web, worker, cron, static_site, private
	RuntimeTier     types.RuntimeTier  `json:"runtime_tier,omitempty"`  // general, isolated, high_memory, gpu
	SourceType      types.SourceType   `json:"source_type,omitempty"`   // git, archive, image
	RepoURL         string             `json:"repo_url,omitempty"`
	ArchiveURL      string             `json:"archive_url,omitempty"`
	ImageURL        string             `json:"image_url,omitempty"`
	Branch          string             `json:"branch,omitempty"`
	RootDirectory   string             `json:"root_directory,omitempty"`
	BuildCommand    string             `json:"build_command,omitempty"`
	StartCommand    string             `json:"start_command,omitempty"`
	DockerfilePath  string             `json:"dockerfile_path,omitempty"`
	Port            int                `json:"port,omitempty"`
	MinReplicas     int                `json:"min_replicas,omitempty"`
	MaxReplicas     int                `json:"max_replicas,omitempty"`
	CPUCores        float64            `json:"cpu_cores,omitempty"`
	MemoryMB        int                `json:"memory_mb,omitempty"`
	DiskMB          int                `json:"disk_mb,omitempty"`
	HealthCheckPath string             `json:"health_check_path,omitempty"`
}

// UpdateServiceRequest holds mutable service fields
type UpdateServiceRequest struct {
	Name            *string            `json:"name,omitempty"`
	Slug            *string            `json:"slug,omitempty"`
	Type            *types.ServiceType `json:"type,omitempty"`
	RuntimeTier     *types.RuntimeTier `json:"runtime_tier,omitempty"`
	SourceType      *types.SourceType  `json:"source_type,omitempty"`
	RepoURL         *string            `json:"repo_url,omitempty"`
	ArchiveURL      *string            `json:"archive_url,omitempty"`
	ImageURL        *string            `json:"image_url,omitempty"`
	Branch          *string            `json:"branch,omitempty"`
	RootDirectory   *string            `json:"root_directory,omitempty"`
	BuildCommand    *string            `json:"build_command,omitempty"`
	StartCommand    *string            `json:"start_command,omitempty"`
	DockerfilePath  *string            `json:"dockerfile_path,omitempty"`
	Port            *int               `json:"port,omitempty"`
	MinReplicas     *int               `json:"min_replicas,omitempty"`
	MaxReplicas     *int               `json:"max_replicas,omitempty"`
	CPUCores        *float64           `json:"cpu_cores,omitempty"`
	MemoryMB        *int               `json:"memory_mb,omitempty"`
	DiskMB          *int               `json:"disk_mb,omitempty"`
	HealthCheckPath *string            `json:"health_check_path,omitempty"`
}

// UploadSourceResponse describes the result of uploading a source code archive
type UploadSourceResponse struct {
	ServiceID  string           `json:"service_id"`
	SourceType types.SourceType `json:"source_type"`
	ArchiveURL string           `json:"archive_url"`
	SizeBytes  int64            `json:"size_bytes"`
	Checksum   string           `json:"checksum"`
	BuildID    string           `json:"build_id,omitempty"`
	Message    string           `json:"message"`
}

// MoveServiceRequest specifies target project (or nil to convert to standalone)
type MoveServiceRequest struct {
	ProjectID *string `json:"project_id"`
}

// EnvVarItem represents a single environment variable to insert or update
type EnvVarItem struct {
	Key       string                `json:"key"`
	Value     string                `json:"value"`
	IsSecret  bool                  `json:"is_secret"`
	Scope     types.EnvVarScope     `json:"scope,omitempty"`
	TargetEnv types.EnvVarTargetEnv `json:"target_env,omitempty"`
	VaultPath string                `json:"vault_path,omitempty"`
}

// SetEnvVarsRequest holds a batch of environment variables to upsert
type SetEnvVarsRequest struct {
	Variables []EnvVarItem `json:"variables"`
}

// ResolvedEnvResponse holds merged environment variables for runtime execution
type ResolvedEnvResponse struct {
	Variables map[string]string `json:"variables"`
}

// RevealSecretResponse returns an unmasked secret value for authorized users
type RevealSecretResponse struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	VaultPath string `json:"vault_path,omitempty"`
}
