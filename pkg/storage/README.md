# Adroit Storage & Archive Engine (`pkg/storage`)

> **Package:** `github.com/dev-nathaniel/Adroit-platform/pkg/storage`  
> **Key Consumers:** `internal/project-service`, `internal/build-service`, CLI tooling

---

## 1. Overview & Purpose

`pkg/storage` provides a unified object persistence and archive normalization abstraction for the Adroit Cloud Platform. It manages source code archives (`.zip`, `.tar.gz`), build contexts, and media assets across local development and distributed cloud infrastructure.

Key capabilities:
* **StorageProvider Abstraction:** Thread-safe, pluggable interface supporting local filesystems, in-memory mock stores, and S3-compatible cloud object storage (AWS S3, Cloudflare R2, MinIO).
* **Safe Archive Normalization:** Converts incoming zip and tar archives into deterministic, clean `.tar.gz` bundles consumable by BuildKit's HTTP tarball context provider (`--opt context=<url>`).
* **Path Traversal Protection (Zip Slip):** Guards against malicious archives containing relative or absolute escape paths (e.g., `../../etc/passwd`).
* **Integrity Hashing:** Streaming SHA-256 calculation for immutability verification and audit trails.

---

## 2. Architecture & Interfaces

### 2.1 The `StorageProvider` Interface

```go
type StorageProvider interface {
    // Save stores the reader contents at key and returns the public or download URL
    Save(ctx context.Context, key string, data io.Reader) (string, error)

    // Delete removes an object by key
    Delete(ctx context.Context, key string) error

    // GetPath returns the local path or remote key representation
    GetPath(key string) string

    // Open opens a read stream for the object along with its byte size
    Open(ctx context.Context, key string) (io.ReadCloser, int64, error)

    // GetURL returns the absolute or relative HTTP download URL for the key
    GetURL(key string) string
}
```

### 2.2 Storage Implementations

| Provider | Constructor | Primary Use Case | Features |
| :--- | :--- | :--- | :--- |
| **`LocalStorage`** | `NewLocalStorage(uploadDir, baseURL)` | Local development & single-node deployments | Thread-safe, atomic path cleaning, automatic directory creation (`0755`), direct file streaming. |
| **`InMemStorage`** | `NewInMemStorage(baseURL)` | Unit & integration testing, CI pipelines | In-memory byte slice map, zero disk I/O, concurrent read/write protection. |
| **`S3Storage`** | `NewS3Storage(cfg)` | Production multi-region deployments | AWS S3, Cloudflare R2, and MinIO compatible using SigV4 HMAC-SHA256 authentication and custom endpoints. |

---

## 3. Archive Normalization Pipeline (`NormalizeToTarGz`)

BuildKit natively consumes HTTP tarballs via `--opt context=http://.../source/download`. To enable seamless deployment from ZIP archives (e.g. from the Adroit CLI or GitHub release artifacts), `pkg/storage` automatically normalizes uploaded archives.

```
Incoming Stream (.zip / .tar.gz)
             │
             ▼
   DetectArchiveType (Magic Bytes)
   - 0x50 0x4B 0x03 (ZIP)
   - 0x1F 0x8B       (GZIP)
   - ustar at 257    (TAR)
             │
             ▼
  Zip Slip & Path Sanitization
  - Rejects ".." traversal attempts
  - Strips leading "/" or "\\" roots
  - Preserves directory structures & permissions
             │
             ▼
 Deterministic Tar & Gzip Compression
             │
             ▼
   NormalizedArchive Output
   - TarGzData: []byte
   - Checksum:  "sha256:abc..."
   - SizeBytes: int64
   - FileCount: int
```

### Path Sanitization & Zip Slip Defense

Any attempt to exploit directory traversal during archive decompression is rejected:
```go
cleanName := filepath.Clean(f.Name)
if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
    return nil, fmt.Errorf("insecure file path in zip archive: %s", f.Name)
}
```

---

## 4. Configuration & Environment Variables

When used in microservices (such as `project-service`), storage backends are configured via environment variables:

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `STORAGE_BACKEND` | String | `local` | Storage provider type: `local`, `s3`, or `memory` |
| `STORAGE_LOCAL_DIR` | String | `data/uploads/sources` | Root directory for local filesystem storage |
| `STORAGE_BASE_URL` | String | `/api/v1/services/source/download` | Base prefix for generated download URLs |
| `STORAGE_S3_ENDPOINT` | String | *(empty)* | Custom S3 endpoint (e.g., MinIO or Cloudflare R2) |
| `STORAGE_S3_REGION` | String | `us-east-1` | S3 region or `auto` for Cloudflare R2 |
| `STORAGE_S3_BUCKET` | String | `adroit-sources` | Target bucket name |
| `STORAGE_S3_ACCESS_KEY` | String | *(empty)* | AWS / R2 / MinIO Access Key ID |
| `STORAGE_S3_SECRET_KEY` | String | *(empty)* | AWS / R2 / MinIO Secret Access Key |
| `STORAGE_S3_PUBLIC_URL` | String | *(empty)* | Optional CDN or public edge domain for downloads |
| `STORAGE_S3_USE_SSL` | Boolean | `true` | Enable HTTPS for S3 API calls |

---

## 5. Usage Examples

### 5.1 Normalizing and Storing an Uploaded Archive

```go
package main

import (
    "context"
    "os"
    "github.com/dev-nathaniel/Adroit-platform/pkg/storage"
)

func handleUpload(provider storage.StorageProvider, file io.Reader, filename string) error {
    // 1. Normalize any .zip or .tar.gz into standard .tar.gz
    normalized, err := storage.NormalizeToTarGz(file, filename)
    if err != nil {
        return err
    }

    // 2. Persist to storage provider
    storageKey := "services/svc-123/source.tar.gz"
    url, err := provider.Save(context.Background(), storageKey, bytes.NewReader(normalized.TarGzData))
    if err != nil {
        return err
    }

    fmt.Printf("Uploaded %d files (%d bytes, checksum %s) to %s\n",
        normalized.FileCount, normalized.SizeBytes, normalized.Checksum, url)
    return nil
}
```

### 5.2 Initializing S3 / Cloudflare R2 Storage

```go
s3Store, err := storage.NewS3Storage(storage.S3Config{
    Endpoint:  "https://<account_id>.r2.cloudflarestorage.com",
    Bucket:    "adroit-sources",
    AccessKey: os.Getenv("R2_ACCESS_KEY"),
    SecretKey: os.Getenv("R2_SECRET_KEY"),
    Region:    "auto",
    UseSSL:    true,
})
```

---

## 6. Testing

Unit tests for `pkg/storage` cover local disk I/O, in-memory read/writes, Zip Slip security validation, format detection, and tar.gz normalization:

```bash
# Run storage tests
go test -v ./pkg/storage/...

# Run with race detection
go test -race ./pkg/storage/...
```
