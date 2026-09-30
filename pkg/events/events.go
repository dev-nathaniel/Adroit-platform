package events

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// NATS JetStream Stream and Subject definitions
const (
	StreamName = "ADROIT_EVENTS"

	// Project events
	SubjectProjectCreated = "adroit.project.created"
	SubjectProjectUpdated = "adroit.project.updated"
	SubjectProjectDeleted = "adroit.project.deleted"

	// Build events
	SubjectBuildRequested = "adroit.build.requested"
	SubjectBuildStarted   = "adroit.build.started"
	SubjectBuildCompleted = "adroit.build.completed"
	SubjectBuildFailed    = "adroit.build.failed"

	// Deployment events
	SubjectDeployRequested = "adroit.deploy.requested"
	SubjectDeployStarted   = "adroit.deploy.started"
	SubjectDeployHealthy   = "adroit.deploy.healthy"
	SubjectDeployFailed    = "adroit.deploy.failed"
	SubjectDeployStopped   = "adroit.deploy.stopped"

	// Database events
	SubjectDatabaseProvisionRequested = "adroit.database.provision_requested"
	SubjectDatabaseProvisioned        = "adroit.database.provisioned"
	SubjectDatabaseDeprovisioned      = "adroit.database.deprovisioned"

	// Service lifecycle events
	SubjectServiceCreated        = "adroit.service.created"
	SubjectServiceUpdated        = "adroit.service.updated"
	SubjectServiceDeleted        = "adroit.service.deleted"
	SubjectServiceMoved          = "adroit.service.moved"
	SubjectServiceScaleRequested = "adroit.service.scale_requested"

	// Organization & User events
	SubjectOrgMemberInvited = "adroit.org.member_invited"
	SubjectOrgMemberJoined  = "adroit.org.member_joined"
	SubjectUserRegistered   = "adroit.user.registered"

	// Notification direct events
	SubjectNotificationEmailSend   = "adroit.notification.email_send"
	SubjectNotificationWebhookSend = "adroit.notification.webhook_send"

	// Git Integration events
	SubjectGitPushReceived    = "adroit.git.push_received"
	SubjectGitWebhookReceived = "adroit.git.webhook_received"
)

// Event is the universal CloudEvents-like envelope sent across NATS JetStream
type Event[T any] struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Data      T         `json:"data"`
}

// New creates an initialized Event envelope with random ID and UTC timestamp
func New[T any](subject, source string, data T) Event[T] {
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	return Event[T]{
		ID:        "evt_" + hex.EncodeToString(bytes),
		Subject:   subject,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}

// BuildRequestedPayload is emitted when a new commit or manual trigger initiates a build
type BuildRequestedPayload struct {
	BuildID        string            `json:"build_id"`
	ServiceID      string            `json:"service_id"`
	ProjectID      string            `json:"project_id"`
	RepoURL        string            `json:"repo_url"`
	Branch         string            `json:"branch"`
	CommitHash     string            `json:"commit_hash"`
	DockerfilePath string            `json:"dockerfile_path,omitempty"`
	BuildArgs      map[string]string `json:"build_args,omitempty"`
}

// BuildCompletedPayload is emitted when BuildKit successfully produces an image
type BuildCompletedPayload struct {
	BuildID     string `json:"build_id"`
	ServiceID   string `json:"service_id"`
	ImageTag    string `json:"image_tag"`
	ImageDigest string `json:"image_digest"`
	DurationMs  int64  `json:"duration_ms"`
}

// DeployRequestedPayload is emitted to request the Orchestration Service to launch on Nomad
type DeployRequestedPayload struct {
	DeploymentID string                  `json:"deployment_id"`
	ServiceID    string                  `json:"service_id"`
	BuildID      string                  `json:"build_id"`
	ImageTag     string                  `json:"image_tag"`
	Port         int                     `json:"port"`
	Replicas     int                     `json:"replicas"`
	Resources    types.ResourceLimits    `json:"resources"`
	HealthCheck  types.HealthCheckConfig `json:"health_check"`
	EnvVars      map[string]string       `json:"env_vars,omitempty"`
	RuntimeTier  types.RuntimeTier       `json:"runtime_tier"`
}

// DeployHealthyPayload is emitted when health-check sidecars confirm application readiness
type DeployHealthyPayload struct {
	DeploymentID string `json:"deployment_id"`
	ServiceID    string `json:"service_id"`
	NomadJobID   string `json:"nomad_job_id"`
	ActiveURL    string `json:"active_url"`
}

// DatabaseProvisionPayload is emitted to initiate a managed database cluster
type DatabaseProvisionPayload struct {
	DatabaseID    string             `json:"database_id"`
	ProjectID     string             `json:"project_id"`
	Name          string             `json:"name"`
	Type          types.DatabaseType `json:"type"`
	Version       string             `json:"version"`
	StorageSizeGB int                `json:"storage_size_gb"`
}

// OrgMemberInvitedPayload is emitted when an invitation is issued for an organization
type OrgMemberInvitedPayload struct {
	InvitationID     string    `json:"invitation_id"`
	OrganizationID   string    `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	InvitedByID      string    `json:"invited_by_id"`
	InvitedByName    string    `json:"invited_by_name"`
	InvitedByEmail   string    `json:"invited_by_email,omitempty"`
	InviteURL        string    `json:"invite_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// EmailNotificationPayload is emitted for direct transactional email dispatch
type EmailNotificationPayload struct {
	To        []string       `json:"to"`
	Subject   string         `json:"subject"`
	Template  string         `json:"template,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	PlainBody string         `json:"plain_body,omitempty"`
	HTMLBody  string         `json:"html_body,omitempty"`
}

// WebhookNotificationPayload is emitted to dispatch a webhook notification
type WebhookNotificationPayload struct {
	URL     string            `json:"url"`
	Event   string            `json:"event"`
	Headers map[string]string `json:"headers,omitempty"`
	Payload any               `json:"payload"`
	Secret  string            `json:"secret,omitempty"`
}

// ProjectCreatedPayload is emitted when a new project is created
type ProjectCreatedPayload struct {
	ProjectID      string `json:"project_id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
}

// ProjectUpdatedPayload is emitted when a project is updated
type ProjectUpdatedPayload struct {
	ProjectID      string `json:"project_id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
}

// ProjectDeletedPayload is emitted when a project is deleted
type ProjectDeletedPayload struct {
	ProjectID      string `json:"project_id"`
	OrganizationID string `json:"organization_id"`
}

// ServiceCreatedPayload is emitted when a service is registered
type ServiceCreatedPayload struct {
	ServiceID      string  `json:"service_id"`
	OrganizationID string  `json:"organization_id"`
	ProjectID      *string `json:"project_id,omitempty"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Type           string  `json:"type"`
	RepoURL        string  `json:"repo_url"`
	Branch         string  `json:"branch"`
}

// ServiceUpdatedPayload is emitted when service config changes
type ServiceUpdatedPayload struct {
	ServiceID      string  `json:"service_id"`
	OrganizationID string  `json:"organization_id"`
	ProjectID      *string `json:"project_id,omitempty"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
}

// ServiceDeletedPayload is emitted when a service is removed
type ServiceDeletedPayload struct {
	ServiceID      string  `json:"service_id"`
	OrganizationID string  `json:"organization_id"`
	ProjectID      *string `json:"project_id,omitempty"`
}

// ServiceMovedPayload is emitted when a service is assigned, moved to, or removed from a project
type ServiceMovedPayload struct {
	ServiceID      string  `json:"service_id"`
	OrganizationID string  `json:"organization_id"`
	OldProjectID   *string `json:"old_project_id,omitempty"`
	NewProjectID   *string `json:"new_project_id,omitempty"`
}

// GitPushReceivedPayload is emitted when a push event is processed by git-integration-service
type GitPushReceivedPayload struct {
	Provider        string   `json:"provider"` // "github" or "gitlab"
	RepoURL         string   `json:"repo_url"`
	Branch          string   `json:"branch"`
	CommitHash      string   `json:"commit_hash"`
	CommitMessage   string   `json:"commit_message"`
	AuthorName      string   `json:"author_name"`
	AuthorEmail     string   `json:"author_email"`
	MatchedServices []string `json:"matched_services"`
	TriggeredBuilds []string `json:"triggered_builds"`
}

// GitWebhookReceivedPayload is emitted for any received Git webhook event
type GitWebhookReceivedPayload struct {
	Provider   string `json:"provider"`
	EventType  string `json:"event_type"`
	DeliveryID string `json:"delivery_id,omitempty"`
	RepoURL    string `json:"repo_url"`
	Status     string `json:"status"`
}


