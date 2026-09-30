package types

import "time"

// ServiceType denotes the kind of workload being deployed
type ServiceType string

const (
	ServiceTypeWeb        ServiceType = "web"
	ServiceTypeWorker     ServiceType = "worker"
	ServiceTypeCron       ServiceType = "cron"
	ServiceTypeStaticSite ServiceType = "static_site"
	ServiceTypePrivate    ServiceType = "private"
)

// RuntimeTier matches the architecture runtime pools
type RuntimeTier string

const (
	RuntimeTierGeneral    RuntimeTier = "general"
	RuntimeTierIsolated   RuntimeTier = "isolated"
	RuntimeTierHighMemory RuntimeTier = "high_memory"
	RuntimeTierGPU        RuntimeTier = "gpu"
)

// BuildStatus tracks the lifecycle of a code compilation/container build
type BuildStatus string

const (
	BuildStatusQueued    BuildStatus = "queued"
	BuildStatusBuilding  BuildStatus = "building"
	BuildStatusSucceeded BuildStatus = "succeeded"
	BuildStatusFailed    BuildStatus = "failed"
	BuildStatusCancelled BuildStatus = "cancelled"
)

// DeploymentStatus tracks workload state on the runtime node
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusDeploying DeploymentStatus = "deploying"
	DeploymentStatusActive    DeploymentStatus = "active"
	DeploymentStatusReplaced  DeploymentStatus = "replaced"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusStopped   DeploymentStatus = "stopped"
)

// DatabaseType defines managed database engines
type DatabaseType string

const (
	DatabaseTypePostgres DatabaseType = "postgres"
	DatabaseTypeRedis    DatabaseType = "redis"
	DatabaseTypeMySQL    DatabaseType = "mysql"
)

// DatabaseStatus tracks state of provisioned stateful clusters
type DatabaseStatus string

const (
	DatabaseStatusProvisioning DatabaseStatus = "provisioning"
	DatabaseStatusRunning      DatabaseStatus = "running"
	DatabaseStatusMaintenance  DatabaseStatus = "maintenance"
	DatabaseStatusStopped      DatabaseStatus = "stopped"
	DatabaseStatusFailed       DatabaseStatus = "failed"
)

// User represents an account holder on the platform
type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	AvatarURL      string    `json:"avatar_url,omitempty"`
	GitHubUsername string    `json:"github_username,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Organization represents a multi-tenant workspace
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InvitationStatus tracks the lifecycle of an organization invite
type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusRevoked  InvitationStatus = "revoked"
	InvitationStatusExpired  InvitationStatus = "expired"
)

// OrganizationInvitation represents a pending or resolved invitation to join a workspace
type OrganizationInvitation struct {
	ID             string           `json:"id"`
	OrganizationID string           `json:"organization_id"`
	Email          string           `json:"email"`
	Role           string           `json:"role"`
	InvitedBy      string           `json:"invited_by"`
	Status         InvitationStatus `json:"status"`
	ExpiresAt      time.Time        `json:"expires_at"`
	CreatedAt      time.Time        `json:"created_at"`
}

// Project groups services and environments together
type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ResourceLimits defines compute limits for isolated execution (microVM / gVisor)
type ResourceLimits struct {
	CPUCores float64 `json:"cpu_cores"` // e.g. 0.5, 1.0, 2.0
	MemoryMB int     `json:"memory_mb"` // e.g. 512, 1024, 2048
	DiskMB   int     `json:"disk_mb"`   // e.g. 5120
	GPUs     int     `json:"gpus,omitempty"`
}

// HealthCheckConfig defines how the runtime verifies app readiness
type HealthCheckConfig struct {
	Path                string `json:"path"`                  // e.g. "/healthz"
	Port                int    `json:"port"`                  // target port
	IntervalSeconds     int    `json:"interval_seconds"`      // interval between checks
	TimeoutSeconds      int    `json:"timeout_seconds"`       // response timeout
	FailureThreshold    int    `json:"failure_threshold"`     // failure limit before marking unhealthy
	InitialDelaySeconds int    `json:"initial_delay_seconds"` // grace period on startup
}

// Service represents a workload within a project
type Service struct {
	ID             string            `json:"id"`
	ProjectID      string            `json:"project_id"`
	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	Type           ServiceType       `json:"type"`
	RuntimeTier    RuntimeTier       `json:"runtime_tier"`
	RepoURL        string            `json:"repo_url"`
	Branch         string            `json:"branch"`
	RootDirectory  string            `json:"root_directory,omitempty"`
	BuildCommand   string            `json:"build_command,omitempty"`
	StartCommand   string            `json:"start_command,omitempty"`
	DockerfilePath string            `json:"dockerfile_path,omitempty"`
	Port           int               `json:"port"`
	MinReplicas    int               `json:"min_replicas"`
	MaxReplicas    int               `json:"max_replicas"`
	Resources      ResourceLimits    `json:"resources"`
	HealthCheck    HealthCheckConfig `json:"health_check"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// Build represents an artifact creation task executed by BuildKit
type Build struct {
	ID          string      `json:"id"`
	ServiceID   string      `json:"service_id"`
	CommitHash  string      `json:"commit_hash"`
	Branch      string      `json:"branch"`
	Message     string      `json:"message"`
	Author      string      `json:"author"`
	Status      BuildStatus `json:"status"`
	ImageTag    string      `json:"image_tag,omitempty"`
	ImageDigest string      `json:"image_digest,omitempty"`
	DurationMs  int64       `json:"duration_ms,omitempty"`
	TriggeredBy string      `json:"triggered_by"`
	CreatedAt   time.Time   `json:"created_at"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
}

// Deployment represents a running version of a service on Nomad
type Deployment struct {
	ID           string           `json:"id"`
	ServiceID    string           `json:"service_id"`
	BuildID      string           `json:"build_id"`
	Status       DeploymentStatus `json:"status"`
	NomadJobID   string           `json:"nomad_job_id,omitempty"`
	ActiveURL    string           `json:"active_url,omitempty"`
	Replicas     int              `json:"replicas"`
	ErrorMessage string           `json:"error_message,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// DatabaseInstance represents a managed stateful database provisioned by the platform
type DatabaseInstance struct {
	ID             string         `json:"id"`
	ProjectID      string         `json:"project_id"`
	Name           string         `json:"name"`
	Type           DatabaseType   `json:"type"`
	Version        string         `json:"version"`
	Status         DatabaseStatus `json:"status"`
	ConnectionURI  string         `json:"connection_uri,omitempty"` // Read-only / injected into services
	StorageSizeGB  int            `json:"storage_size_gb"`
	Resources      ResourceLimits `json:"resources"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Domain represents a custom domain pointing to a service via Caddy edge
type Domain struct {
	ID         string    `json:"id"`
	ServiceID  string    `json:"service_id"`
	DomainName string    `json:"domain_name"`
	Verified   bool      `json:"verified"`
	SSLActive  bool      `json:"ssl_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
