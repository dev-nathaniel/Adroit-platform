package client

import "time"

// User represents an authenticated account.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Organization represents a workspace or team.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthResponse is returned on successful registration or login.
type AuthResponse struct {
	Token        string        `json:"token"`
	User         User          `json:"user"`
	Organization *Organization `json:"organization,omitempty"`
}

// Project represents a collection of services.
type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	ServiceCount   int       `json:"service_count,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Service represents an individual deployable workload.
type Service struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organization_id"`
	ProjectID          *string   `json:"project_id,omitempty"`
	Name               string    `json:"name"`
	Type               string    `json:"type"` // "web", "worker", "cron", "redis", "postgres"
	SourceType         string    `json:"source_type"` // "archive", "git", "image"
	Port               int       `json:"port"`
	RepoURL            string    `json:"repo_url,omitempty"`
	Branch             string    `json:"branch,omitempty"`
	ImageURL           string    `json:"image_url,omitempty"`
	ArchiveURL         string    `json:"archive_url,omitempty"`
	CurrentReplicas    int       `json:"current_replicas,omitempty"`
	ActiveDeploymentID *string   `json:"active_deployment_id,omitempty"`
	ActiveURL          string    `json:"active_url,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Deployment represents a rollout iteration of a service.
type Deployment struct {
	ID          string    `json:"id"`
	ServiceID   string    `json:"service_id"`
	BuildID     *string   `json:"build_id,omitempty"`
	Status      string    `json:"status"` // "pending", "building", "deploying", "active", "failed", "cancelled"
	Strategy    string    `json:"strategy"` // "rolling", "canary", "blue-green"
	Replicas    int       `json:"replicas"`
	ActiveURL   string    `json:"active_url,omitempty"`
	ImageTag    string    `json:"image_tag,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Allocation represents an instance of a workload running in Nomad.
type Allocation struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	NomadAllocID string    `json:"nomad_alloc_id"`
	Name         string    `json:"name"`
	ClientStatus string    `json:"client_status"` // "running", "complete", "failed"
	Desired      string    `json:"desired_status"`
	NodeName     string    `json:"node_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Build represents a container compilation job.
type Build struct {
	ID           string     `json:"id"`
	ServiceID    string     `json:"service_id"`
	CommitHash   string     `json:"commit_hash,omitempty"`
	Status       string     `json:"status"` // "queued", "building", "succeeded", "failed"
	ErrorMessage string     `json:"error_message,omitempty"`
	DurationMs   int64      `json:"duration_ms,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// EnvVar represents an environment variable or secret.
type EnvVar struct {
	ID        string    `json:"id,omitempty"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	IsSecret  bool      `json:"is_secret"`
	Scope     string    `json:"scope"` // "runtime", "build", "all"
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// CustomDomain represents an ingress domain mapped to a service.
type CustomDomain struct {
	ID           string    `json:"id"`
	ServiceID    string    `json:"service_id"`
	Domain       string    `json:"domain"`
	Status       string    `json:"status"` // "pending", "verified", "active", "failed"
	DNSRecordType string   `json:"dns_record_type"`
	TargetValue  string    `json:"target_value"`
	CreatedAt    time.Time `json:"created_at"`
}

// UploadResponse is returned by POST /api/v1/services/{id}/upload.
type UploadResponse struct {
	ServiceID    string `json:"service_id"`
	ArchiveURL   string `json:"archive_url"`
	SizeBytes    int64  `json:"size_bytes"`
	SHA256       string `json:"sha256"`
	BuildID      string `json:"build_id,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
}
