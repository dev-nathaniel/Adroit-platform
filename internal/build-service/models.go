package buildservice

import (
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// TriggerBuildRequest represents the payload to trigger a manual build
type TriggerBuildRequest struct {
	CommitHash     string            `json:"commit_hash,omitempty"`
	Branch         string            `json:"branch,omitempty"`
	Message        string            `json:"message,omitempty"`
	DockerfilePath string            `json:"dockerfile_path,omitempty"`
	BuildArgs      map[string]string `json:"build_args,omitempty"`
}

// BuildListResponse wraps a paginated list of builds
type BuildListResponse struct {
	Builds []types.Build `json:"builds"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// BuildLogsResponse contains streamed or persisted log entries and concatenated text
type BuildLogsResponse struct {
	BuildID    string                `json:"build_id"`
	Logs       []types.BuildLogEntry `json:"logs"`
	FullLog    string                `json:"full_log"`
	TotalLines int                   `json:"total_lines"`
}

// BuildDetail includes the build and summary of service it belongs to
type BuildDetail struct {
	types.Build
	ServiceName string `json:"service_name,omitempty"`
	ServiceSlug string `json:"service_slug,omitempty"`
}

// BuildLogEntryDTO is a helper for streaming log items
type BuildLogEntryDTO struct {
	Line      string    `json:"line"`
	Stream    string    `json:"stream"`
	Timestamp time.Time `json:"timestamp"`
}
