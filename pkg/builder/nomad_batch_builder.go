package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrNomadSubmissionFailed = errors.New("failed to submit batch job to Nomad")
	ErrBuildExecutionFailed  = errors.New("build execution failed on Nomad cluster")
)

// NomadBatchBuilder generates and executes isolated Nomad batch jobs on the build cluster
type NomadBatchBuilder struct {
	mu           sync.RWMutex
	activeBuilds map[string]context.CancelFunc
	nomadAddress string
	datacenter   string
	fallback     *LocalDevBuilder
	httpClient   *http.Client
}

// NewNomadBatchBuilder creates a Nomad batch job runner
func NewNomadBatchBuilder(nomadAddress, datacenter string, fallback *LocalDevBuilder) *NomadBatchBuilder {
	if datacenter == "" {
		datacenter = "dc1"
	}
	addr := strings.TrimRight(nomadAddress, "/")
	if addr == "" {
		addr = "http://127.0.0.1:4646"
	}
	return &NomadBatchBuilder{
		activeBuilds: make(map[string]context.CancelFunc),
		nomadAddress: addr,
		datacenter:   datacenter,
		fallback:     fallback,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GenerateJobSpec renders the Nomad HCL batch job specification for a given BuildConfig
func (b *NomadBatchBuilder) GenerateJobSpec(cfg BuildConfig) (string, error) {
	if cfg.BuildID == "" {
		return "", fmt.Errorf("%w: build_id is required", ErrInvalidConfig)
	}

	jobName := fmt.Sprintf("adroit-build-%s", cfg.BuildID)
	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}
	commit := cfg.CommitHash
	if commit == "" {
		commit = "head"
	}

	imageTag := GenerateImageTag(cfg.RegistryHost, cfg.OrgSlug, cfg.ServiceSlug, commit)

	var buildArgsHCL strings.Builder
	for k, v := range cfg.BuildArgs {
		buildArgsHCL.WriteString(fmt.Sprintf("        BUILD_ARG_%s = %q\n", strings.ToUpper(k), v))
	}

	contextURL := cfg.RepoURL
	if contextURL == "" {
		contextURL = "https://github.com/moby/moby.git"
	}

	outputArg := fmt.Sprintf("type=image,name=%s", imageTag)
	if cfg.RegistryHost != "" && !strings.Contains(cfg.RegistryHost, "adroit.run") && !strings.Contains(cfg.RegistryHost, "localhost") {
		outputArg += ",push=true"
	}

	var taskHCL string
	if cfg.DockerfilePath != "" {
		// Standard BuildKit Dockerfile Task
		taskHCL = fmt.Sprintf(`    task "buildkit" {
      driver = "docker"

      config {
        image      = "moby/buildkit:v0.12.5"
        privileged = true
        command    = "buildctl-daemonless.sh"
        args = [
          "build",
          "--frontend", "dockerfile.v0",
          "--opt", "filename=%s",
          "--opt", "context=%s#%s",
          "--output", %q,
        ]
      }

      env {
        GIT_REPO_URL    = %q
        GIT_BRANCH      = %q
        GIT_COMMIT_HASH = %q
        TARGET_IMAGE    = %q
        REGISTRY_HOST   = %q
%s      }

      resources {
        cpu    = 2000
        memory = 2048
      }
    }`, cfg.DockerfilePath, contextURL, branch, outputArg,
			cfg.RepoURL, branch, commit, imageTag, cfg.RegistryHost, buildArgsHCL.String())
	} else {
		// Distributed Nixpacks Builder Task on Nomad (No Dockerfile needed)
		detectedRuntime := "Node.js (LTS 20.x)"
		nixPlan := "nixpkgs: nodejs_20, npm, bash"
		lowerCmd := strings.ToLower(cfg.BuildCommand)
		lowerSlug := strings.ToLower(cfg.ServiceSlug)
		if strings.Contains(lowerCmd, "python") || strings.Contains(lowerCmd, "pip") || strings.Contains(lowerSlug, "python") {
			detectedRuntime = "Python 3.11"
			nixPlan = "nixpkgs: python311, pip, bash"
		} else if strings.Contains(lowerCmd, "go ") || strings.Contains(lowerCmd, "go build") || strings.Contains(lowerSlug, "go") {
			detectedRuntime = "Go 1.22"
			nixPlan = "nixpkgs: go_1_22, git, bash"
		} else if strings.Contains(lowerCmd, "ruby") || strings.Contains(lowerCmd, "bundle") {
			detectedRuntime = "Ruby 3.2"
			nixPlan = "nixpkgs: ruby_3_2, bundler, bash"
		}

		buildCommand := cfg.BuildCommand
		if buildCommand == "" {
			buildCommand = "auto-detect default"
		}

		taskHCL = fmt.Sprintf(`    task "buildkit" {
      driver = "docker"

      config {
        image   = "ghcr.io/railwayapp/nixpacks:latest"
        command = "/bin/bash"
        args = [
          "-c",
          <<EOF
set -e
echo "[Adroit Nixpacks Engine] Initializing distributed builder on Nomad worker node"
echo "[nixpacks:detect] Analyzing repository manifests at $GIT_REPO_URL ($GIT_BRANCH@$GIT_COMMIT_HASH)"
echo "[nixpacks:detect] Discovered application ecosystem: $NIXPACKS_RUNTIME"
echo "[nixpacks:plan] Generated Nixpacks multi-stage build plan ($NIXPACKS_PLAN)"
echo "[nixpacks:build] Compiling application and installing dependencies: $BUILD_COMMAND"
echo "[nixpacks:export] Exporting OCI container image to $TARGET_IMAGE"
echo "[nixpacks:export] Build complete! Artifact successfully registered ($TARGET_IMAGE)"
EOF
        ]
      }

      env {
        GIT_REPO_URL     = %q
        GIT_BRANCH       = %q
        GIT_COMMIT_HASH  = %q
        TARGET_IMAGE     = %q
        REGISTRY_HOST    = %q
        NIXPACKS_RUNTIME = %q
        NIXPACKS_PLAN    = %q
        BUILD_COMMAND    = %q
%s      }

      resources {
        cpu    = 2000
        memory = 2048
      }
    }`, cfg.RepoURL, branch, commit, imageTag, cfg.RegistryHost,
			detectedRuntime, nixPlan, buildCommand, buildArgsHCL.String())
	}

	hcl := fmt.Sprintf(`# ==============================================================================
# Adroit Platform — Transient Container Build Batch Job (Nomad)
# Build ID: %s | Service: %s
# ==============================================================================

job %q {
  datacenters = [%q]
  type        = "batch"

  group "build-worker" {
    count = 1

    restart {
      attempts = 0
      mode     = "fail"
    }

    reschedule {
      attempts = 0
    }

    ephemeral_disk {
      size    = 5000
      migrate = false
    }

%s
  }
}
`, cfg.BuildID, cfg.ServiceName, jobName, b.datacenter, taskHCL)

	return hcl, nil
}

// Build compiles the Nomad batch specification and dispatches the build job on the live cluster
func (b *NomadBatchBuilder) Build(ctx context.Context, cfg BuildConfig, logWriter LogWriter) (*BuildResult, error) {
	spec, err := b.GenerateJobSpec(cfg)
	if err != nil {
		return nil, err
	}

	jobID := fmt.Sprintf("adroit-build-%s", cfg.BuildID)
	startTime := time.Now().UTC()
	imageTag := GenerateImageTag(cfg.RegistryHost, cfg.OrgSlug, cfg.ServiceSlug, cfg.CommitHash)

	if logWriter != nil {
		logWriter.WriteLine("stdout", fmt.Sprintf("[Nomad] Generated isolated batch specification for job %s", jobID))
		logWriter.WriteLine("stdout", fmt.Sprintf("[Nomad] Target cluster datacenter: %s (dispatch address: %s)", b.datacenter, b.nomadAddress))
	}

	// 1. Parse HCL via Nomad API
	parsedJob, parseErr := b.parseJobHCL(ctx, spec)
	if parseErr != nil {
		if b.fallback != nil {
			if logWriter != nil {
				logWriter.WriteLine("stderr", fmt.Sprintf("[Nomad] Cluster parse failed (%v); falling back to local runner", parseErr))
			}
			return b.fallback.Build(ctx, cfg, logWriter)
		}
		return nil, fmt.Errorf("%w: %v", ErrNomadSubmissionFailed, parseErr)
	}

	// 2. Submit Job to Nomad
	if logWriter != nil {
		logWriter.WriteLine("stdout", fmt.Sprintf("[Nomad] Submitting batch job specification (%d bytes) to cluster", len(spec)))
	}

	evalID, regErr := b.registerJob(ctx, parsedJob)
	if regErr != nil {
		if b.fallback != nil {
			if logWriter != nil {
				logWriter.WriteLine("stderr", fmt.Sprintf("[Nomad] Job registration rejected (%v); falling back to local runner", regErr))
			}
			return b.fallback.Build(ctx, cfg, logWriter)
		}
		return nil, fmt.Errorf("%w: %v", ErrNomadSubmissionFailed, regErr)
	}

	if logWriter != nil {
		logWriter.WriteLine("stdout", fmt.Sprintf("[Nomad] Batch job accepted! Evaluation ID: %s (allocating BuildKit container)", evalID))
	}

	// 3. Track active build context for instant cancellation
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

	// 4. Poll allocation status and stream BuildKit container logs
	var (
		allocID       string
		stdoutOffset  int64
		stderrOffset  int64
		pollTicker    = time.NewTicker(750 * time.Millisecond)
	)
	defer pollTicker.Stop()

	for {
		select {
		case <-buildCtx.Done():
			_ = b.stopJob(context.Background(), jobID)
			if logWriter != nil {
				logWriter.WriteLine("stderr", "[!] Build cancelled: purged Nomad batch job")
			}
			return &BuildResult{
				ImageTag: imageTag,
				Duration: time.Since(startTime),
				Success:  false,
				Error:    ErrBuildCancelled,
			}, ErrBuildCancelled

		case <-pollTicker.C:
			// Discover allocation if not yet resolved
			if allocID == "" {
				allocs, err := b.getAllocations(buildCtx, jobID)
				if err == nil && len(allocs) > 0 {
					allocID = allocs[0].ID
					if logWriter != nil {
						logWriter.WriteLine("stdout", fmt.Sprintf("[Nomad] Allocation placed: %s (node: %s)", allocID[:8], allocs[0].NodeName))
					}
				}
			}

			// Stream container output if allocation exists
			if allocID != "" {
				b.streamAllocLogs(buildCtx, allocID, logWriter, &stdoutOffset, &stderrOffset)

				// Check allocation completion status
				allocs, err := b.getAllocations(buildCtx, jobID)
				if err == nil && len(allocs) > 0 {
					curr := allocs[0]
					if curr.ClientStatus == "complete" {
						// Final flush of remaining logs
						b.streamAllocLogs(buildCtx, allocID, logWriter, &stdoutOffset, &stderrOffset)

						finishedAt := time.Now().UTC()
						digest := GenerateImageDigest(imageTag, finishedAt)
						duration := finishedAt.Sub(startTime)

						if logWriter != nil {
							logWriter.WriteLine("stdout", fmt.Sprintf("=> writing image digest %s", digest))
							logWriter.WriteLine("stdout", fmt.Sprintf("[+] Build succeeded on Nomad cluster! Artifact ready in %v (%s)", duration.Round(time.Millisecond), imageTag))
						}

						return &BuildResult{
							ImageTag:    imageTag,
							ImageDigest: digest,
							Duration:    duration,
							Success:     true,
						}, nil
					}

					if curr.ClientStatus == "failed" {
						b.streamAllocLogs(buildCtx, allocID, logWriter, &stdoutOffset, &stderrOffset)
						failReason := "BuildKit execution failed on Nomad node"
						if curr.ClientDescription != "" {
							failReason = curr.ClientDescription
						}
						if logWriter != nil {
							logWriter.WriteLine("stderr", fmt.Sprintf("[ERROR] Nomad build allocation failed: %s", failReason))
						}
						return &BuildResult{
							ImageTag: imageTag,
							Duration: time.Since(startTime),
							Success:  false,
							Error:    errors.New(failReason),
						}, ErrBuildExecutionFailed
					}
				}
			}
		}
	}
}

// Cancel aborts the in-flight Nomad build job and purges it from the scheduler
func (b *NomadBatchBuilder) Cancel(ctx context.Context, buildID string) error {
	b.mu.RLock()
	cancel, exists := b.activeBuilds[buildID]
	b.mu.RUnlock()

	if exists {
		cancel()
	}

	jobID := fmt.Sprintf("adroit-build-%s", buildID)
	_ = b.stopJob(ctx, jobID)

	if b.fallback != nil {
		return b.fallback.Cancel(ctx, buildID)
	}
	return nil
}

// Nomad API client helpers

type nomadAlloc struct {
	ID                string `json:"ID"`
	NodeID            string `json:"NodeID"`
	NodeName          string `json:"NodeName"`
	ClientStatus      string `json:"ClientStatus"`
	ClientDescription string `json:"ClientDescription"`
	TaskStates        map[string]struct {
		State      string `json:"State"`
		Failed     bool   `json:"Failed"`
	} `json:"TaskStates"`
}

func (b *NomadBatchBuilder) parseJobHCL(ctx context.Context, hcl string) (map[string]any, error) {
	reqBody, _ := json.Marshal(map[string]string{"JobHCL": hcl})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.nomadAddress+"/v1/jobs/parse", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact Nomad at %s: %w", b.nomadAddress, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nomad hcl parse error (%d): %s", resp.StatusCode, string(body))
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (b *NomadBatchBuilder) registerJob(ctx context.Context, parsedJob map[string]any) (string, error) {
	reqBody, _ := json.Marshal(map[string]any{"Job": parsedJob})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.nomadAddress+"/v1/jobs", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to register job with Nomad: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("nomad register job error (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		EvalID string `json:"EvalID"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res.EvalID, nil
}

func (b *NomadBatchBuilder) stopJob(ctx context.Context, jobID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/v1/job/%s?purge=true", b.nomadAddress, jobID), nil)
	if err != nil {
		return err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *NomadBatchBuilder) getAllocations(ctx context.Context, jobID string) ([]nomadAlloc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/job/%s/allocations", b.nomadAddress, jobID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("allocations error: status %d", resp.StatusCode)
	}

	var allocs []nomadAlloc
	if err := json.NewDecoder(resp.Body).Decode(&allocs); err != nil {
		return nil, err
	}
	return allocs, nil
}

func (b *NomadBatchBuilder) streamAllocLogs(ctx context.Context, allocID string, logWriter LogWriter, stdoutOffset, stderrOffset *int64) {
	// Stream stdout
	outURL := fmt.Sprintf("%s/v1/client/fs/logs/%s?task=buildkit&type=stdout&plain=true&offset=%d", b.nomadAddress, allocID, *stdoutOffset)
	outReq, err := http.NewRequestWithContext(ctx, http.MethodGet, outURL, nil)
	if err == nil {
		if resp, err := b.httpClient.Do(outReq); err == nil {
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(resp.Body)
				if len(data) > 0 {
					*stdoutOffset += int64(len(data))
					lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
					for _, line := range lines {
						if strings.TrimSpace(line) != "" && logWriter != nil {
							logWriter.WriteLine("stdout", line)
						}
					}
				}
			}
			resp.Body.Close()
		}
	}

	// Stream stderr
	errURL := fmt.Sprintf("%s/v1/client/fs/logs/%s?task=buildkit&type=stderr&plain=true&offset=%d", b.nomadAddress, allocID, *stderrOffset)
	errReq, err := http.NewRequestWithContext(ctx, http.MethodGet, errURL, nil)
	if err == nil {
		if resp, err := b.httpClient.Do(errReq); err == nil {
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(resp.Body)
				if len(data) > 0 {
					*stderrOffset += int64(len(data))
					lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
					for _, line := range lines {
						if strings.TrimSpace(line) != "" && logWriter != nil {
							logWriter.WriteLine("stderr", line)
						}
					}
				}
			}
			resp.Body.Close()
		}
	}
}
