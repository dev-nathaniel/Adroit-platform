package userservice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// StorageProvider abstracts persistence of media and static assets
type StorageProvider interface {
	Save(ctx context.Context, filename string, data io.Reader) (string, error)
	Delete(ctx context.Context, filename string) error
	GetPath(filename string) string
	Open(filename string) (io.ReadCloser, int64, error)
}

// LocalStorage stores files on the local filesystem
type LocalStorage struct {
	uploadDir string
	baseURL   string
}

// NewLocalStorage initializes local filesystem storage.
func NewLocalStorage(uploadDir, baseURL string) (*LocalStorage, error) {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "data/uploads/avatars"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory %s: %w", uploadDir, err)
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "/api/v1/users/avatars"
	}

	return &LocalStorage{
		uploadDir: uploadDir,
		baseURL:   baseURL,
	}, nil
}

// Save writes the incoming stream to disk and returns the public URL
func (s *LocalStorage) Save(ctx context.Context, filename string, data io.Reader) (string, error) {
	cleanName := filepath.Base(filepath.Clean(filename))
	destPath := filepath.Join(s.uploadDir, cleanName)

	file, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file on disk: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, data); err != nil {
		return "", fmt.Errorf("failed to write file content: %w", err)
	}

	return fmt.Sprintf("%s/%s", s.baseURL, cleanName), nil
}

// Delete removes a file from disk
func (s *LocalStorage) Delete(ctx context.Context, filename string) error {
	cleanName := filepath.Base(filepath.Clean(filename))
	path := filepath.Join(s.uploadDir, cleanName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetPath returns the local filesystem path for a file
func (s *LocalStorage) GetPath(filename string) string {
	cleanName := filepath.Base(filepath.Clean(filename))
	return filepath.Join(s.uploadDir, cleanName)
}

// Open opens a file from disk for streaming
func (s *LocalStorage) Open(filename string) (io.ReadCloser, int64, error) {
	cleanName := filepath.Base(filepath.Clean(filename))
	f, err := os.Open(filepath.Join(s.uploadDir, cleanName))
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// InMemStorage implements StorageProvider in memory (ideal for tests)
type InMemStorage struct {
	mu      sync.RWMutex
	baseURL string
	files   map[string][]byte
}

// NewInMemStorage initializes in-memory storage
func NewInMemStorage(baseURL string) *InMemStorage {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "/api/v1/users/avatars"
	}
	return &InMemStorage{
		baseURL: baseURL,
		files:   make(map[string][]byte),
	}
}

// Save stores file in memory
func (m *InMemStorage) Save(ctx context.Context, filename string, data io.Reader) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	content, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}
	cleanName := filepath.Base(filepath.Clean(filename))
	m.files[cleanName] = content
	return fmt.Sprintf("%s/%s", m.baseURL, cleanName), nil
}

// Delete removes file from memory
func (m *InMemStorage) Delete(ctx context.Context, filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanName := filepath.Base(filepath.Clean(filename))
	delete(m.files, cleanName)
	return nil
}

// GetPath returns filename in memory
func (m *InMemStorage) GetPath(filename string) string {
	return filepath.Base(filepath.Clean(filename))
}

// Open opens reader over in-memory file
func (m *InMemStorage) Open(filename string) (io.ReadCloser, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanName := filepath.Base(filepath.Clean(filename))
	data, ok := m.files[cleanName]
	if !ok {
		return nil, 0, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

// Get retrieves raw bytes (helper for test assertions)
func (m *InMemStorage) Get(filename string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.files[filepath.Base(filepath.Clean(filename))]
	return b, ok
}
