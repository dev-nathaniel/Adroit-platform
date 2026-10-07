package builder

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

type memoryLogWriter struct {
	mu    sync.Mutex
	lines []string
}

func (m *memoryLogWriter) WriteLine(stream, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, line)
}

func (m *memoryLogWriter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lines)
}

func TestGenerateImageTag(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		org          string
		service      string
		commit       string
		expectedTag  string
	}{
		{
			name:        "Standard tag",
			host:        "registry.adroit.run",
			org:         "acme-corp",
			service:     "web-api",
			commit:      "a1b2c3d4e5f67890",
			expectedTag: "registry.adroit.run/acme-corp/web-api:a1b2c3d4e5f6",
		},
		{
			name:        "Defaults on empty",
			host:        "",
			org:         "",
			service:     "",
			commit:      "",
			expectedTag: "registry.adroit.run/default/app:latest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GenerateImageTag(tc.host, tc.org, tc.service, tc.commit)
			if got != tc.expectedTag {
				t.Errorf("expected %s, got %s", tc.expectedTag, got)
			}
		})
	}
}

func TestLocalDevBuilder_Success(t *testing.T) {
	b := NewLocalDevBuilder(0) // 0 duration for fast tests
	logs := &memoryLogWriter{}

	cfg := BuildConfig{
		BuildID:        "bld-12345",
		ServiceID:      "svc-67890",
		ServiceName:    "frontend",
		ServiceSlug:    "frontend",
		OrgSlug:        "my-team",
		RepoURL:        "https://github.com/my-team/frontend.git",
		Branch:         "main",
		CommitHash:     "1234567890abcdef",
		DockerfilePath: "Dockerfile.prod",
		BuildCommand:   "npm run build",
		BuildArgs: map[string]string{
			"NODE_ENV": "production",
		},
	}

	ctx := context.Background()
	result, err := b.Build(ctx, cfg, logs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected build to succeed")
	}

	if !strings.HasPrefix(result.ImageTag, "registry.adroit.run/my-team/frontend:") {
		t.Errorf("unexpected image tag: %s", result.ImageTag)
	}

	if !strings.HasPrefix(result.ImageDigest, "sha256:") {
		t.Errorf("unexpected digest: %s", result.ImageDigest)
	}

	if logs.Count() == 0 {
		t.Errorf("expected logs to be written, got 0")
	}
}

func TestLocalDevBuilder_Cancellation(t *testing.T) {
	b := NewLocalDevBuilder(20 * time.Millisecond) // non-zero to allow cancel to catch it
	logs := &memoryLogWriter{}

	cfg := BuildConfig{
		BuildID:        "bld-cancel-test",
		ServiceID:      "svc-cancel",
		ServiceSlug:    "svc-cancel",
		OrgSlug:        "org",
		RepoURL:        "https://github.com/org/svc.git",
		CommitHash:     "abcdef123456",
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 10ms
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := b.Build(ctx, cfg, logs)
	if err == nil {
		t.Fatalf("expected build to fail with cancellation error")
	}

	if err != ErrBuildCancelled && err != context.Canceled {
		t.Errorf("expected ErrBuildCancelled or context.Canceled, got %v", err)
	}
}

func TestLocalDevBuilder_CancelMethod(t *testing.T) {
	b := NewLocalDevBuilder(50 * time.Millisecond)
	logs := &memoryLogWriter{}

	cfg := BuildConfig{
		BuildID:     "bld-cancel-method",
		ServiceID:   "svc-cancel-2",
		ServiceSlug: "svc-cancel-2",
		OrgSlug:     "org",
		CommitHash:  "112233445566",
	}

	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = b.Cancel(context.Background(), "bld-cancel-method")
	}()

	_, err := b.Build(context.Background(), cfg, logs)
	if err == nil {
		t.Fatalf("expected build to be cancelled")
	}
}

func TestNomadBatchBuilder_GenerateJobSpec(t *testing.T) {
	local := NewLocalDevBuilder(0)
	nomad := NewNomadBatchBuilder("http://nomad.service.consul:4646", "dc1", local)

	cfg := BuildConfig{
		BuildID:        "bld-nomad-999",
		ServiceID:      "svc-nomad",
		ServiceName:    "nomad-api",
		ServiceSlug:    "nomad-api",
		OrgSlug:        "enterprise",
		RepoURL:        "https://github.com/enterprise/nomad-api.git",
		Branch:         "v2.0",
		CommitHash:     "9876543210ab",
		DockerfilePath: "deployments/Dockerfile",
		BuildArgs: map[string]string{
			"GO_VERSION": "1.24",
		},
	}

	spec, err := nomad.GenerateJobSpec(cfg)
	if err != nil {
		t.Fatalf("failed to generate job spec: %v", err)
	}

	if !strings.Contains(spec, `job "adroit-build-bld-nomad-999"`) {
		t.Errorf("missing job name in spec: %s", spec)
	}

	if !strings.Contains(spec, `type        = "batch"`) {
		t.Errorf("missing batch type in spec")
	}

	if !strings.Contains(spec, `BUILD_ARG_GO_VERSION = "1.24"`) {
		t.Errorf("missing build arg in spec")
	}
}

func TestNomadBatchBuilder_LiveNomadSubmission(t *testing.T) {
	nomad := NewNomadBatchBuilder("http://localhost:4646", "dc1", nil)

	cfg := BuildConfig{
		BuildID:        "bld-live-batch-001",
		ServiceID:      "svc-live-batch",
		ServiceName:    "live-batch-service",
		ServiceSlug:    "live-batch-service",
		OrgSlug:        "acme",
		RepoURL:        "https://github.com/Dev-Nathaniel/test-repo.git",
		Branch:         "main",
		CommitHash:     "1234567890ab",
		DockerfilePath: "Dockerfile",
	}

	spec, err := nomad.GenerateJobSpec(cfg)
	if err != nil {
		t.Fatalf("failed to generate job spec: %v", err)
	}

	parsed, err := nomad.parseJobHCL(context.Background(), spec)
	if err != nil {
		t.Fatalf("live Nomad HCL parse failed: %v", err)
	}

	evalID, err := nomad.registerJob(context.Background(), parsed)
	if err != nil {
		t.Fatalf("live Nomad batch job registration failed: %v", err)
	}

	t.Logf("Live Nomad batch job submitted successfully! EvalID: %s", evalID)

	// Clean up / purge test batch job
	_ = nomad.stopJob(context.Background(), "adroit-build-bld-live-batch-001")
}

func TestNomadBatchBuilder_ArchiveSourceSpec(t *testing.T) {
	nomad := NewNomadBatchBuilder("http://127.0.0.1:4646", "dc1", nil)

	cfg := BuildConfig{
		BuildID:        "bld-archive-001",
		ServiceID:      "svc-archive",
		ServiceName:    "archive-app",
		ServiceSlug:    "archive-app",
		OrgSlug:        "my-org",
		SourceType:     types.SourceTypeArchive,
		ArchiveURL:     "https://storage.adroit.run/sources/svc-archive/bundle-abc123.tar.gz",
		DockerfilePath: "Dockerfile",
	}

	spec, err := nomad.GenerateJobSpec(cfg)
	if err != nil {
		t.Fatalf("failed to generate job spec: %v", err)
	}

	// 1. Must contain Nomad artifact stanza downloading archive
	if !strings.Contains(spec, `artifact {`) || !strings.Contains(spec, `source      = "https://storage.adroit.run/sources/svc-archive/bundle-abc123.tar.gz"`) {
		t.Errorf("missing artifact stanza for archive download in spec:\n%s", spec)
	}

	// 2. BuildKit context option must be the archive URL
	if !strings.Contains(spec, `"--opt", "context=https://storage.adroit.run/sources/svc-archive/bundle-abc123.tar.gz"`) {
		t.Errorf("missing BuildKit archive context option in spec:\n%s", spec)
	}
}

func TestNomadBatchBuilder_ImageSource_BypassesNomad(t *testing.T) {
	// Nomad pointing to dummy offline address - must NOT make any HTTP calls
	nomad := NewNomadBatchBuilder("http://invalid-nomad-host:9999", "dc1", nil)

	cfg := BuildConfig{
		BuildID:     "bld-image-001",
		ServiceID:   "svc-image",
		ServiceName: "postgres-svc",
		ServiceSlug: "postgres-svc",
		OrgSlug:     "my-org",
		SourceType:  types.SourceTypeImage,
		ImageURL:    "docker.io/library/postgres:16-alpine",
	}

	res, err := nomad.Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("expected image source to bypass Nomad build and succeed, got err: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success to be true")
	}
	if res.ImageTag != "docker.io/library/postgres:16-alpine" {
		t.Errorf("expected image tag postgres:16-alpine, got %s", res.ImageTag)
	}
	if res.Duration != 0 {
		t.Errorf("expected 0 duration for pre-built image, got %v", res.Duration)
	}
}

func TestLocalDevBuilder_ImageSource(t *testing.T) {
	local := NewLocalDevBuilder(0)

	cfg := BuildConfig{
		BuildID:     "bld-local-img",
		ServiceID:   "svc-img",
		ServiceName: "redis-cache",
		ServiceSlug: "redis-cache",
		OrgSlug:     "my-org",
		SourceType:  types.SourceTypeImage,
		ImageURL:    "redis:7.2-alpine",
	}

	res, err := local.Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success")
	}
	if res.ImageTag != "redis:7.2-alpine" {
		t.Errorf("expected redis:7.2-alpine, got %s", res.ImageTag)
	}
}

func TestLocalDevBuilder_ArchiveSource(t *testing.T) {
	local := NewLocalDevBuilder(0)

	var logLines []string
	logs := FuncLogWriter(func(stream, line string) {
		logLines = append(logLines, line)
	})

	cfg := BuildConfig{
		BuildID:        "bld-local-archive",
		ServiceID:      "svc-archive",
		ServiceName:    "archive-app",
		ServiceSlug:    "archive-app",
		OrgSlug:        "my-org",
		SourceType:     types.SourceTypeArchive,
		ArchiveURL:     "http://localhost:8080/api/v1/services/source/download/app.tar.gz",
		DockerfilePath: "Dockerfile",
	}

	res, err := local.Build(context.Background(), cfg, logs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success")
	}

	var hasArchiveContext bool
	for _, l := range logLines {
		if strings.Contains(l, "archive context") {
			hasArchiveContext = true
			break
		}
	}
	if !hasArchiveContext {
		t.Errorf("expected log to mention archive context, got logs: %v", logLines)
	}
}

