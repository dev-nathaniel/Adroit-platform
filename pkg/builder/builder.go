package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBuildCancelled   = errors.New("build was cancelled")
	ErrBuildTimeout     = errors.New("build exceeded maximum duration timeout")
	ErrInvalidConfig    = errors.New("invalid build configuration")
	ErrBuilderNotFound  = errors.New("builder runner not found")
)

// BuildConfig defines the parameters needed to execute a container build
type BuildConfig struct {
	BuildID        string            `json:"build_id"`
	ServiceID      string            `json:"service_id"`
	ServiceName    string            `json:"service_name"`
	ServiceSlug    string            `json:"service_slug"`
	OrgSlug        string            `json:"org_slug"`
	RepoURL        string            `json:"repo_url"`
	Branch         string            `json:"branch"`
	CommitHash     string            `json:"commit_hash"`
	DockerfilePath string            `json:"dockerfile_path,omitempty"`
	BuildCommand   string            `json:"build_command,omitempty"`
	RootDirectory  string            `json:"root_directory,omitempty"`
	BuildArgs      map[string]string `json:"build_args,omitempty"`
	RegistryHost   string            `json:"registry_host,omitempty"`
	Timeout        time.Duration     `json:"timeout,omitempty"`
}

// BuildResult captures the outcome of a build execution
type BuildResult struct {
	ImageTag    string        `json:"image_tag"`
	ImageDigest string        `json:"image_digest"`
	Duration    time.Duration `json:"duration"`
	Success     bool          `json:"success"`
	Error       error         `json:"error,omitempty"`
}

// LogWriter receives log lines emitted during build execution
type LogWriter interface {
	WriteLine(stream, line string)
}

// FuncLogWriter adapts a function into a LogWriter
type FuncLogWriter func(stream, line string)

func (f FuncLogWriter) WriteLine(stream, line string) {
	if f != nil {
		f(stream, line)
	}
}

// Builder defines the pluggable contract for executing builds
type Builder interface {
	// Build executes the compilation/container build task synchronously, streaming logs via logWriter
	Build(ctx context.Context, cfg BuildConfig, logWriter LogWriter) (*BuildResult, error)
	// Cancel aborts an in-flight build by ID
	Cancel(ctx context.Context, buildID string) error
}

// GenerateImageTag returns the canonical image tag: registry.adroit.run/<org>/<service>:<commit>
func GenerateImageTag(registryHost, orgSlug, serviceSlug, commitHash string) string {
	if registryHost == "" {
		registryHost = "registry.adroit.run"
	}
	orgSlug = sanitizeSlug(orgSlug, "default")
	serviceSlug = sanitizeSlug(serviceSlug, "app")

	tag := commitHash
	if len(tag) > 12 {
		tag = tag[:12]
	}
	if tag == "" {
		tag = "latest"
	}

	return fmt.Sprintf("%s/%s/%s:%s", registryHost, orgSlug, serviceSlug, tag)
}

// GenerateImageDigest creates a reproducible deterministic or unique SHA256 image digest
func GenerateImageDigest(imageTag string, timestamp time.Time) string {
	data := fmt.Sprintf("%s-%d", imageTag, timestamp.UnixNano())
	hash := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func sanitizeSlug(s, fallback string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return fallback
	}
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	res := strings.Trim(sb.String(), "-")
	if res == "" {
		return fallback
	}
	return res
}
