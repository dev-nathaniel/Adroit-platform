package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrFileNotFound = errors.New("file not found in storage")
	ErrInvalidKey   = errors.New("invalid storage key")
)

// StorageProvider abstracts persistence of media, build archives, and static assets
type StorageProvider interface {
	Save(ctx context.Context, key string, data io.Reader) (string, error)
	Delete(ctx context.Context, key string) error
	GetPath(key string) string
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	GetURL(key string) string
}

// ============================================================================
// LOCAL STORAGE
// ============================================================================

// LocalStorage stores files on the local filesystem
type LocalStorage struct {
	mu        sync.RWMutex
	uploadDir string
	baseURL   string
}

// NewLocalStorage initializes local filesystem storage.
func NewLocalStorage(uploadDir, baseURL string) (*LocalStorage, error) {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "data/uploads/sources"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory %s: %w", uploadDir, err)
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "/api/v1/services/source/download"
	}

	return &LocalStorage{
		uploadDir: uploadDir,
		baseURL:   baseURL,
	}, nil
}

// Save writes the incoming stream to disk and returns the public/download URL
func (s *LocalStorage) Save(ctx context.Context, key string, data io.Reader) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanKey := filepath.Clean(key)
	if strings.HasPrefix(cleanKey, "..") || strings.HasPrefix(cleanKey, "/") || strings.HasPrefix(cleanKey, "\\") {
		cleanKey = filepath.Base(cleanKey)
	}

	destPath := filepath.Join(s.uploadDir, cleanKey)
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory %s: %w", dir, err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file on disk: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, data); err != nil {
		return "", fmt.Errorf("failed to write file content: %w", err)
	}

	return s.GetURL(cleanKey), nil
}

// Delete removes a file from disk
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanKey := filepath.Clean(key)
	path := filepath.Join(s.uploadDir, cleanKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetPath returns the local filesystem path for a file
func (s *LocalStorage) GetPath(key string) string {
	cleanKey := filepath.Clean(key)
	return filepath.Join(s.uploadDir, cleanKey)
}

// Open opens a file from disk for streaming
func (s *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanKey := filepath.Clean(key)
	destPath := filepath.Join(s.uploadDir, cleanKey)
	f, err := os.Open(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrFileNotFound
		}
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// GetURL returns the canonical download URL for key
func (s *LocalStorage) GetURL(key string) string {
	cleanKey := filepath.ToSlash(filepath.Clean(key))
	if strings.HasPrefix(s.baseURL, "http://") || strings.HasPrefix(s.baseURL, "https://") {
		return fmt.Sprintf("%s/%s", s.baseURL, cleanKey)
	}
	return fmt.Sprintf("%s/%s", s.baseURL, cleanKey)
}

// ============================================================================
// IN-MEMORY STORAGE (IDEAL FOR TESTS)
// ============================================================================

// InMemStorage implements StorageProvider in memory
type InMemStorage struct {
	mu      sync.RWMutex
	files   map[string][]byte
	baseURL string
}

// NewInMemStorage initializes in-memory storage
func NewInMemStorage(baseURL string) *InMemStorage {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "/api/v1/services/source/download"
	}
	return &InMemStorage{
		files:   make(map[string][]byte),
		baseURL: baseURL,
	}
}

func (m *InMemStorage) Save(ctx context.Context, key string, data io.Reader) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}

	cleanKey := filepath.ToSlash(filepath.Clean(key))
	m.files[cleanKey] = buf
	return m.GetURL(cleanKey), nil
}

func (m *InMemStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanKey := filepath.ToSlash(filepath.Clean(key))
	delete(m.files, cleanKey)
	return nil
}

func (m *InMemStorage) GetPath(key string) string {
	return filepath.ToSlash(filepath.Clean(key))
}

func (m *InMemStorage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanKey := filepath.ToSlash(filepath.Clean(key))
	buf, ok := m.files[cleanKey]
	if !ok {
		return nil, 0, ErrFileNotFound
	}
	return io.NopCloser(bytes.NewReader(buf)), int64(len(buf)), nil
}

func (m *InMemStorage) GetURL(key string) string {
	cleanKey := filepath.ToSlash(filepath.Clean(key))
	return fmt.Sprintf("%s/%s", m.baseURL, cleanKey)
}

// Get retrieves raw bytes from memory
func (m *InMemStorage) Get(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cleanKey := filepath.ToSlash(filepath.Clean(key))
	buf, ok := m.files[cleanKey]
	return buf, ok
}

// ============================================================================
// S3 / MINIO / R2 COMPATIBLE STORAGE
// ============================================================================

// S3Config holds connection options for S3, Cloudflare R2, or MinIO
type S3Config struct {
	Endpoint      string // e.g. "https://<account-id>.r2.cloudflarestorage.com" or "http://minio:9000"
	Bucket        string // bucket name
	AccessKey     string
	SecretKey     string
	Region        string // e.g. "auto" or "us-east-1"
	PublicBaseURL string // optional CDN / custom domain for downloads
	UseSSL        bool
}

// S3Storage provides object persistence on AWS S3, Cloudflare R2, or MinIO
type S3Storage struct {
	cfg        S3Config
	httpClient *http.Client
}

// NewS3Storage creates an S3/R2/MinIO storage provider
func NewS3Storage(cfg S3Config) *S3Storage {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	return &S3Storage{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Save uploads the file to S3/R2 bucket
func (s *S3Storage) Save(ctx context.Context, key string, data io.Reader) (string, error) {
	cleanKey := strings.TrimLeft(filepath.ToSlash(filepath.Clean(key)), "/")

	buf, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("failed to read data stream: %w", err)
	}

	reqURL := fmt.Sprintf("%s/%s/%s", s.cfg.Endpoint, s.cfg.Bucket, cleanKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("failed to construct s3 request: %w", err)
	}

	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(buf)))

	// Sign request if credentials provided
	if s.cfg.AccessKey != "" && s.cfg.SecretKey != "" {
		s.signV4(req, buf)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload object to S3/R2: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("s3 upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return s.GetURL(cleanKey), nil
}

// Delete removes an object from S3/R2
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	cleanKey := strings.TrimLeft(filepath.ToSlash(filepath.Clean(key)), "/")
	reqURL := fmt.Sprintf("%s/%s/%s", s.cfg.Endpoint, s.cfg.Bucket, cleanKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}
	if s.cfg.AccessKey != "" && s.cfg.SecretKey != "" {
		s.signV4(req, nil)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetPath returns the object key / URI
func (s *S3Storage) GetPath(key string) string {
	cleanKey := strings.TrimLeft(filepath.ToSlash(filepath.Clean(key)), "/")
	return fmt.Sprintf("s3://%s/%s", s.cfg.Bucket, cleanKey)
}

// Open fetches the object stream from S3/R2
func (s *S3Storage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	cleanKey := strings.TrimLeft(filepath.ToSlash(filepath.Clean(key)), "/")
	reqURL := fmt.Sprintf("%s/%s/%s", s.cfg.Endpoint, s.cfg.Bucket, cleanKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	if s.cfg.AccessKey != "" && s.cfg.SecretKey != "" {
		s.signV4(req, nil)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, 0, ErrFileNotFound
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, 0, fmt.Errorf("s3 get failed with status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, resp.ContentLength, nil
}

// GetURL returns public or CDN URL for the object
func (s *S3Storage) GetURL(key string) string {
	cleanKey := strings.TrimLeft(filepath.ToSlash(filepath.Clean(key)), "/")
	if s.cfg.PublicBaseURL != "" {
		baseURL := strings.TrimRight(s.cfg.PublicBaseURL, "/")
		return fmt.Sprintf("%s/%s", baseURL, cleanKey)
	}
	return fmt.Sprintf("%s/%s/%s", s.cfg.Endpoint, s.cfg.Bucket, cleanKey)
}

// signV4 calculates standard AWS Signature V4 authorization header
func (s *S3Storage) signV4(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)

	var payloadHash string
	if len(payload) > 0 {
		h := sha256.Sum256(payload)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		h := sha256.Sum256([]byte(""))
		payloadHash = hex.EncodeToString(h[:])
	}
	req.Header.Set("x-amz-content-sha256", payloadHash)

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.URL.Host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method, canonicalURI, req.URL.RawQuery, canonicalHeaders, signedHeaders, payloadHash)

	hReq := sha256.Sum256([]byte(canonicalRequest))
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.cfg.Region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate, credentialScope, hex.EncodeToString(hReq[:]))

	kDate := hmacSHA256([]byte("AWS4"+s.cfg.SecretKey), dateStamp)
	kRegion := hmacSHA256(kDate, s.cfg.Region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.cfg.AccessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// ============================================================================
// ENVIRONMENT FACTORY
// ============================================================================

// NewStorageProviderFromEnv returns an initialized storage provider based on environment variables
func NewStorageProviderFromEnv(defaultDir, baseURL string) StorageProvider {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND")))

	if backend == "memory" {
		return NewInMemStorage(baseURL)
	}

	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	region := os.Getenv("S3_REGION")
	publicBaseURL := os.Getenv("STORAGE_PUBLIC_BASE_URL")

	if backend == "s3" || backend == "r2" || (bucket != "" && accessKey != "") {
		if endpoint == "" {
			endpoint = "https://s3.amazonaws.com"
		}
		if region == "" {
			region = "us-east-1"
		}
		useSSL := !strings.HasPrefix(endpoint, "http://")
		return NewS3Storage(S3Config{
			Endpoint:      endpoint,
			Bucket:        bucket,
			AccessKey:     accessKey,
			SecretKey:     secretKey,
			Region:        region,
			PublicBaseURL: publicBaseURL,
			UseSSL:        useSSL,
		})
	}

	localStore, err := NewLocalStorage(defaultDir, baseURL)
	if err != nil {
		return NewInMemStorage(baseURL)
	}
	return localStore
}
