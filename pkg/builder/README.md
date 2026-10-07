# Adroit Pluggable Build Engine (`pkg/builder`)

> **Package:** `github.com/dev-nathaniel/Adroit-platform/pkg/builder`  
> **Key Consumers:** `internal/build-service`

---

## 1. Overview & Purpose

`pkg/builder` provides a modular container image compilation engine for the Adroit Cloud Platform. It decouples the build lifecycle management (tracking state in PostgreSQL and coordinating events over NATS) from the underlying build execution technology.

Key capabilities:
* **`Builder` Interface:** Standardized execution interface for initiating container builds, streaming stdout/stderr in real time, and handling atomic cancellations.
* **Dual Context Ingestion:** Compiles OCI images from Git repositories (`RepoURL`) or remote HTTP tarballs (`ArchiveURL`, powered by `pkg/storage`).
* **Rootless BuildKit Compatibility:** Seamless execution with Moby BuildKit daemon syntax (`--opt context=...`, `--opt filename=Dockerfile`).
* **Image Tagging & Immutability:** Deterministic SHA-256 digest generation and registry canonical tagging (`<registry>/<org>/<service>:<commit>`).

---

## 2. Architecture & The `Builder` Interface

```go
type Builder interface {
    // Type returns the unique identifier of the runner engine ("local" or "nomad")
    Type() string

    // Build starts compilation with the provided configuration and returns a log channel
    Build(ctx context.Context, cfg BuildConfig) (<-chan BuildLogChunk, <-chan BuildResult, error)

    // Cancel gracefully terminates an in-flight build task
    Cancel(ctx context.Context, buildID string) error
}
```

### 2.1 The `BuildConfig` Specification

```go
type BuildConfig struct {
    BuildID        string            `json:"build_id"`
    ServiceID      string            `json:"service_id"`
    ServiceName    string            `json:"service_name"`
    ServiceSlug    string            `json:"service_slug"`
    OrgSlug        string            `json:"org_slug"`
    SourceType     types.SourceType  `json:"source_type,omitempty"` // "git", "archive", "image"
    RepoURL        string            `json:"repo_url,omitempty"`
    ArchiveURL     string            `json:"archive_url,omitempty"`
    Branch         string            `json:"branch,omitempty"`
    CommitHash     string            `json:"commit_hash,omitempty"`
    DockerfilePath string            `json:"dockerfile_path,omitempty"`
    BuildCommand   string            `json:"build_command,omitempty"`
    BuildArgs      map[string]string `json:"build_args,omitempty"`
    RegistryHost   string            `json:"registry_host,omitempty"`
    Timeout        time.Duration     `json:"timeout,omitempty"`
}
```

---

## 3. Runner Engines

### 3.1 Local Dev Runner (`LocalDevBuilder`)
* **Target:** Local workstation development, automated unit tests, and single-host setups.
* **Mechanism:** Simulates BuildKit's multi-stage pipeline steps (context download, Dockerfile parse, step execution, image registry push).
* **Context Resolution:** Clones Git repository or downloads the HTTP tarball context from `ArchiveURL`.
* **Zero Dependencies:** Runs natively in Go without requiring a live Nomad cluster or external BuildKit daemon.

### 3.2 Nomad Batch Runner (`NomadBatchBuilder`)
* **Target:** Multi-tenant production cloud clusters.
* **Mechanism:** Renders dynamic Nomad HCL2 batch jobs (`task "buildkit"`).
* **Dockerfile Builds:** Runs `moby/buildkit:v0.12.5-rootless` with:
  - For Git sources: `--opt context=git://...`
  - For Archive sources: `--opt context=http://.../api/v1/services/<id>/source/download`
* **Zero-Config Nixpacks Builds (`ghcr.io/railwayapp/nixpacks:latest`):**
  - When `DockerfilePath == ""`, the builder dynamically schedules a transient Nixpacks batch task on Nomad.
  - Automatically identifies runtime stacks (Node.js LTS, Python 3.11, Go 1.22, Ruby 3.2, etc.).
  - Executes build plans without requiring developers to write or maintain Dockerfiles.
* **Resource Isolation:** Dedicated CPU (2000 MHz), RAM (2048 MB), and ephemeral disk quotas per build run to protect cluster neighbors.
* **Registry Handoff:** Pushes compiled images directly to Harbor or platform internal container registry.

---

## 4. Zero-Config Nixpacks Compilation Pipeline

When a service is created without specifying a `dockerfile_path`:

```
Source Ingestion (Git Repo or Uploaded Archive)
                      │
                      ▼
            Inspect Repository Files
                      │
   ┌──────────────────┴──────────────────┐
   │                                     │
Dockerfile Exists               No Dockerfile Found
   │                                     │
   ▼                                     ▼
BuildKit Engine                  Nixpacks Engine
(moby/buildkit:rootless)         (ghcr.io/railwayapp/nixpacks:latest)
   │                                     │
   │                                     ├─ [1. Detect]  Identify runtime & dependencies
   │                                     ├─ [2. Plan]    Generate Nix multi-stage plan
   │                                     ├─ [3. Build]   Compile binaries & dependencies
   │                                     └─ [4. Export]  Produce standardized OCI image
   │                                                     │
   └──────────────────┬──────────────────────────────────┘
                      │
                      ▼
         Canonical OCI Container Tag
  registry.adroit.run/<org>/<service>:<commit>
```

---

## 5. Usage Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/dev-nathaniel/Adroit-platform/pkg/builder"
    "github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

func runBuild(b builder.Builder) error {
    cfg := builder.BuildConfig{
        BuildID:        "bld-12345",
        ServiceID:      "svc-67890",
        ServiceName:    "api-gateway",
        ServiceSlug:    "api-gateway",
        OrgSlug:        "acme-corp",
        SourceType:     types.SourceTypeArchive,
        ArchiveURL:     "http://localhost:8084/api/v1/services/svc-67890/source/download",
        DockerfilePath: "Dockerfile",
        RegistryHost:   "registry.adroit.run",
    }

    logCh, resCh, err := b.Build(context.Background(), cfg)
    if err != nil {
        return err
    }

    // Stream logs
    go func() {
        for chunk := range logCh {
            fmt.Print(chunk.Text)
        }
    }()

    result := <-resCh
    if result.Error != nil {
        return fmt.Errorf("build failed: %w", result.Error)
    }

    fmt.Printf("Build succeeded! Tag: %s, Digest: %s\n", result.ImageTag, result.Digest)
    return nil
}
```

---

## 5. Testing

```bash
# Run unit tests
go test -v ./pkg/builder/...

# Run with race detector
go test -race ./pkg/builder/...
```
