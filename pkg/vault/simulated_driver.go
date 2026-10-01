package vault

import (
	"context"
	"fmt"
	"sync"
)

// SimulatedSecretStore provides an in-memory thread-safe implementation of SecretStore
type SimulatedSecretStore struct {
	mu   sync.RWMutex
	data map[string]map[string]string // key: path, val: key-value map
}

// NewSimulatedSecretStore creates an in-memory secret store for testing and local development
func NewSimulatedSecretStore() *SimulatedSecretStore {
	return &SimulatedSecretStore{
		data: make(map[string]map[string]string),
	}
}

// VaultPath returns simulated logical path
func (s *SimulatedSecretStore) VaultPath(orgID string, scope ScopeType, entityID string) string {
	return fmt.Sprintf("adroit/%s/%s/%s", orgID, scope, entityID)
}

// PutSecrets merges secrets into in-memory store
func (s *SimulatedSecretStore) PutSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string, secrets map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.VaultPath(orgID, scope, entityID)
	if _, exists := s.data[path]; !exists {
		s.data[path] = make(map[string]string)
	}
	for k, v := range secrets {
		s.data[path][k] = v
	}
	return nil
}

// GetSecrets returns copies of all secrets for an entity
func (s *SimulatedSecretStore) GetSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.VaultPath(orgID, scope, entityID)
	store, exists := s.data[path]
	if !exists {
		return make(map[string]string), nil
	}

	result := make(map[string]string, len(store))
	for k, v := range store {
		result[k] = v
	}
	return result, nil
}

// GetSecretKey retrieves a single secret value
func (s *SimulatedSecretStore) GetSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.VaultPath(orgID, scope, entityID)
	store, exists := s.data[path]
	if !exists {
		return "", ErrSecretNotFound
	}
	val, ok := store[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}

// DeleteSecretKey removes a specific secret key
func (s *SimulatedSecretStore) DeleteSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.VaultPath(orgID, scope, entityID)
	if store, exists := s.data[path]; exists {
		delete(store, key)
		if len(store) == 0 {
			delete(s.data, path)
		}
	}
	return nil
}

// DeleteEntitySecrets wipes all secrets for an entity
func (s *SimulatedSecretStore) DeleteEntitySecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.VaultPath(orgID, scope, entityID)
	delete(s.data, path)
	return nil
}

// SnapshotDeployment creates a deployment snapshot in memory
func (s *SimulatedSecretStore) SnapshotDeployment(ctx context.Context, orgID, deploymentID string, secrets map[string]string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.VaultPath(orgID, ScopeDeployment, deploymentID)
	s.data[path] = make(map[string]string, len(secrets))
	for k, v := range secrets {
		s.data[path][k] = v
	}
	return path, nil
}
