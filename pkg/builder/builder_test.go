package builder

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
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
