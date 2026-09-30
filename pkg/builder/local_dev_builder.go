package builder

import (
	"context"
	"fmt"
	"sync"
	"time"
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

	// Normalize defaults
	dockerfile := cfg.DockerfilePath
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}
	commit := cfg.CommitHash
	if commit == "" {
		commit = "head"
	}

	imageTag := GenerateImageTag(cfg.RegistryHost, cfg.OrgSlug, cfg.ServiceSlug, commit)

	writeLog := func(stream, line string) {
		if logWriter != nil {
			logWriter.WriteLine(stream, line)
		}
	}
	logf := func(stream, format string, args ...any) {
		writeLog(stream, fmt.Sprintf(format, args...))
	}

	// Step sequence
	type step struct {
		name   string
		stream string
	}

	steps := []step{
		{name: "[+] Building with BuildKit engine (Adroit Local Runner)", stream: "stdout"},
		{name: fmt.Sprintf("=> [internal] load build definition from %s", dockerfile), stream: "stdout"},
		{name: "=> [internal] load .dockerignore", stream: "stdout"},
		{name: "=> [internal] load metadata for docker.io/library/alpine:latest", stream: "stdout"},
		{name: fmt.Sprintf("=> [internal] load build context from %s (%s@%s)", cfg.RepoURL, branch, commit), stream: "stdout"},
	}

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
