package builder

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// LocalDevBuilder simulates or executes local Docker/BuildKit builds
// It emits realistic structured logs, tracks running builds, and supports instant cancellation.
type LocalDevBuilder struct {
	mu           sync.RWMutex
	activeBuilds map[string]context.CancelFunc
	stepDuration time.Duration // artificial pause per step (0 for tests)
}

// NewLocalDevBuilder creates an instance of LocalDevBuilder
func NewLocalDevBuilder(stepDuration time.Duration) *LocalDevBuilder {
	return &LocalDevBuilder{
		activeBuilds: make(map[string]context.CancelFunc),
		stepDuration: stepDuration,
	}
}

// Build executes the local development build simulation with full BuildKit-style logging
func (b *LocalDevBuilder) Build(ctx context.Context, cfg BuildConfig, logWriter LogWriter) (*BuildResult, error) {
	if cfg.BuildID == "" {
		return nil, fmt.Errorf("%w: build_id is required", ErrInvalidConfig)
	}

	writeLog := func(stream, line string) {
		if logWriter != nil {
			logWriter.WriteLine(stream, line)
		}
	}

	// If pre-built image source, skip build step entirely
	if cfg.SourceType == types.SourceTypeImage {
		imageTag := cfg.ImageURL
		if imageTag == "" {
			imageTag = cfg.RepoURL
		}
		if imageTag == "" {
			imageTag = "latest"
		}
		writeLog("stdout", fmt.Sprintf("[Image] Pre-built container image specified: %s", imageTag))
		writeLog("stdout", "=> skipping BuildKit compilation step (pre-built image)")
		writeLog("stdout", fmt.Sprintf("=> image artifact verified: %s", imageTag))
		return &BuildResult{
			ImageTag:    imageTag,
			ImageDigest: GenerateImageDigest(imageTag, time.Now().UTC()),
			Duration:    0,
			Success:     true,
		}, nil
	}

	buildCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	b.mu.Lock()
	b.activeBuilds[cfg.BuildID] = cancel
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.activeBuilds, cfg.BuildID)
		b.mu.Unlock()
	}()

	startTime := time.Now().UTC()

	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}
	commit := cfg.CommitHash
	if commit == "" {
		commit = "head"
	}

	imageTag := GenerateImageTag(cfg.RegistryHost, cfg.OrgSlug, cfg.ServiceSlug, commit)
	logf := func(stream, format string, args ...any) {
		writeLog(stream, fmt.Sprintf(format, args...))
	}

	// Step sequence
	type step struct {
		name   string
		stream string
	}

	var steps []step
	if cfg.DockerfilePath == "" {
		// Cloud Native Buildpack (CNB) Detection & Lifecycle
		detectedRuntime := "Node.js (LTS 20.x)"
		buildpackID := "paketo-buildpacks/nodejs"
		lowerCmd := strings.ToLower(cfg.BuildCommand)
		lowerSlug := strings.ToLower(cfg.ServiceSlug)
		if strings.Contains(lowerCmd, "python") || strings.Contains(lowerCmd, "pip") || strings.Contains(lowerSlug, "python") {
			detectedRuntime = "Python 3.11"
			buildpackID = "paketo-buildpacks/python"
		} else if strings.Contains(lowerCmd, "go ") || strings.Contains(lowerCmd, "go build") || strings.Contains(lowerSlug, "go") {
			detectedRuntime = "Go 1.22"
			buildpackID = "paketo-buildpacks/go"
		} else if strings.Contains(lowerCmd, "ruby") || strings.Contains(lowerCmd, "bundle") {
			detectedRuntime = "Ruby 3.2"
			buildpackID = "paketo-buildpacks/ruby"
		}

		sourceDesc := fmt.Sprintf("repository %s (%s@%s)", cfg.RepoURL, branch, commit)
		if cfg.SourceType == types.SourceTypeArchive || cfg.ArchiveURL != "" {
			sourceDesc = fmt.Sprintf("uploaded archive %s", cfg.ArchiveURL)
		}

		steps = append(steps,
			step{name: "[+] No Dockerfile found: Invoking Cloud Native Buildpacks (CNB) auto-detection", stream: "stdout"},
			step{name: fmt.Sprintf("=> [buildpack:detect] Inspecting %s", sourceDesc), stream: "stdout"},
			step{name: fmt.Sprintf("=> [buildpack:detect] Identified application ecosystem: %s", detectedRuntime), stream: "stdout"},
			step{name: fmt.Sprintf("=> [buildpack:plan] Selected buildpack pipeline: %s", buildpackID), stream: "stdout"},
			step{name: "=> [buildpack:build] Setting up hermetic build environment and fetching base layers", stream: "stdout"},
		)
		if cfg.BuildCommand != "" {
			steps = append(steps, step{
				name:   fmt.Sprintf("=> [build-cmd] RUN %s", cfg.BuildCommand),
				stream: "stdout",
			})
		} else {
			steps = append(steps, step{
				name:   fmt.Sprintf("=> [buildpack:build] Running auto-detected build for %s", detectedRuntime),
				stream: "stdout",
			})
		}
		for k, v := range cfg.BuildArgs {
			steps = append(steps, step{
				name:   fmt.Sprintf("=> [build-arg] ARG %s=%s", k, v),
				stream: "stdout",
			})
		}
		steps = append(steps,
			step{name: "=> [buildpack:export] Packaging runtime artifact into OCI standard container", stream: "stdout"},
			step{name: "=> [buildpack:export] Configuring process type 'web' with zero-configuration entrypoint", stream: "stdout"},
			step{name: "=> exporting to image", stream: "stdout"},
			step{name: fmt.Sprintf("=> => naming to %s", imageTag), stream: "stdout"},
		)
	} else {
		contextDesc := fmt.Sprintf("%s (%s@%s)", cfg.RepoURL, branch, commit)
		if cfg.SourceType == types.SourceTypeArchive || cfg.ArchiveURL != "" {
			contextDesc = fmt.Sprintf("archive context %s", cfg.ArchiveURL)
		}

		steps = append(steps,
			step{name: "[+] Building with BuildKit engine (Adroit Local Runner)", stream: "stdout"},
			step{name: fmt.Sprintf("=> [internal] load build definition from %s", cfg.DockerfilePath), stream: "stdout"},
			step{name: "=> [internal] load .dockerignore", stream: "stdout"},
			step{name: "=> [internal] load metadata for docker.io/library/alpine:latest", stream: "stdout"},
			step{name: fmt.Sprintf("=> [internal] load build context from %s", contextDesc), stream: "stdout"},
		)
		if cfg.BuildCommand != "" {
			steps = append(steps, step{
				name:   fmt.Sprintf("=> [build-cmd] RUN %s", cfg.BuildCommand),
				stream: "stdout",
			})
		}
		for k, v := range cfg.BuildArgs {
			steps = append(steps, step{
				name:   fmt.Sprintf("=> [build-arg] ARG %s=%s", k, v),
				stream: "stdout",
			})
		}
		steps = append(steps,
			step{name: "=> [stage-0 1/2] compiling application and caching container layers", stream: "stdout"},
			step{name: "=> [stage-0 2/2] stripping debug symbols and optimizing final artifact", stream: "stdout"},
			step{name: "=> exporting to image", stream: "stdout"},
			step{name: fmt.Sprintf("=> => naming to %s", imageTag), stream: "stdout"},
		)
	}

	for _, s := range steps {
		select {
		case <-buildCtx.Done():
			writeLog("stderr", "[!] Build cancelled or aborted")
			return &BuildResult{
				ImageTag: imageTag,
				Duration: time.Since(startTime),
				Success:  false,
				Error:    ErrBuildCancelled,
			}, ErrBuildCancelled
		default:
		}

		writeLog(s.stream, s.name)

		if b.stepDuration > 0 {
			select {
			case <-buildCtx.Done():
				writeLog("stderr", "[!] Build cancelled or aborted during execution")
				return &BuildResult{
					ImageTag: imageTag,
					Duration: time.Since(startTime),
					Success:  false,
					Error:    ErrBuildCancelled,
				}, ErrBuildCancelled
			case <-time.After(b.stepDuration):
			}
		}
	}

	finishedAt := time.Now().UTC()
	digest := GenerateImageDigest(imageTag, finishedAt)
	duration := finishedAt.Sub(startTime)

	logf("stdout", "=> => writing image %s", digest)
	logf("stdout", "[+] Build succeeded! Artifact ready in %v (%s)", duration.Round(time.Millisecond), imageTag)

	return &BuildResult{
		ImageTag:    imageTag,
		ImageDigest: digest,
		Duration:    duration,
		Success:     true,
	}, nil
}

// Cancel terminates a running build by its buildID
func (b *LocalDevBuilder) Cancel(ctx context.Context, buildID string) error {
	b.mu.RLock()
	cancel, exists := b.activeBuilds[buildID]
	b.mu.RUnlock()

	if !exists {
		// Build may have already completed or was not in memory
		return nil
	}

	cancel()
	return nil
}
