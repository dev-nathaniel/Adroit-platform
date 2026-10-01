package vault

import (
	"context"
	"fmt"
)

// ScopeType represents the entity type that owns the secrets
type ScopeType string

const (
	ScopeService    ScopeType = "services"
	ScopeProject    ScopeType = "projects"
	ScopeDeployment ScopeType = "deployments"
)

// SecretStore provides an abstracted interface for secret storage and retrieval
type SecretStore interface {
	// PutSecrets writes or merges secret key-value pairs for an entity
	PutSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string, secrets map[string]string) error

	// GetSecrets retrieves all secret key-value pairs for an entity
	GetSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) (map[string]string, error)

	// GetSecretKey retrieves a single secret value by key
	GetSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) (string, error)

	// DeleteSecretKey removes a specific secret key from the entity
	DeleteSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) error

	// DeleteEntitySecrets wipes all secrets for an entity
	DeleteEntitySecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) error

	// SnapshotDeployment duplicates resolved secrets to an immutable deployment snapshot path
	SnapshotDeployment(ctx context.Context, orgID, deploymentID string, secrets map[string]string) (string, error)

	// VaultPath returns the logical path within Vault for a given entity
	VaultPath(orgID string, scope ScopeType, entityID string) string
}

// VaultSecretStore implements SecretStore backed by a live HashiCorp Vault cluster
type VaultSecretStore struct {
	client *VaultHTTPClient
}

// NewVaultSecretStore initializes a VaultSecretStore
func NewVaultSecretStore(client *VaultHTTPClient) *VaultSecretStore {
	return &VaultSecretStore{client: client}
}

// VaultPath returns the standard KV v2 path: adroit/{orgID}/{scope}/{entityID}
func (s *VaultSecretStore) VaultPath(orgID string, scope ScopeType, entityID string) string {
	return fmt.Sprintf("adroit/%s/%s/%s", orgID, scope, entityID)
}

// PutSecrets merges secrets into Vault KV v2
func (s *VaultSecretStore) PutSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string, secrets map[string]string) error {
	path := s.VaultPath(orgID, scope, entityID)
	// Read existing secrets to perform a merge
	existing, err := s.client.GetSecret(ctx, path)
	if err != nil && err != ErrSecretNotFound {
		return fmt.Errorf("failed to read existing secrets from vault: %w", err)
	}
	if existing == nil {
		existing = make(map[string]string)
	}

	for k, v := range secrets {
		existing[k] = v
	}

	return s.client.PutSecret(ctx, path, existing)
}

// GetSecrets reads all secrets from Vault KV v2
func (s *VaultSecretStore) GetSecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) (map[string]string, error) {
	path := s.VaultPath(orgID, scope, entityID)
	secrets, err := s.client.GetSecret(ctx, path)
	if err != nil {
		if err == ErrSecretNotFound {
			return make(map[string]string), nil
		}
		return nil, err
	}
	return secrets, nil
}

// GetSecretKey retrieves a single secret value
func (s *VaultSecretStore) GetSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) (string, error) {
	secrets, err := s.GetSecrets(ctx, orgID, scope, entityID)
	if err != nil {
		return "", err
	}
	val, ok := secrets[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return val, nil
}

// DeleteSecretKey removes a specific key by rewriting the map without it
func (s *VaultSecretStore) DeleteSecretKey(ctx context.Context, orgID string, scope ScopeType, entityID string, key string) error {
	path := s.VaultPath(orgID, scope, entityID)
	existing, err := s.client.GetSecret(ctx, path)
	if err != nil {
		if err == ErrSecretNotFound {
			return nil
		}
		return err
	}

	if _, exists := existing[key]; !exists {
		return nil
	}

	delete(existing, key)
	if len(existing) == 0 {
		return s.client.DeleteSecret(ctx, path)
	}
	return s.client.PutSecret(ctx, path, existing)
}

// DeleteEntitySecrets wipes all secrets for an entity
func (s *VaultSecretStore) DeleteEntitySecrets(ctx context.Context, orgID string, scope ScopeType, entityID string) error {
	path := s.VaultPath(orgID, scope, entityID)
	return s.client.DeleteSecret(ctx, path)
}

// SnapshotDeployment stores an immutable snapshot of deployment secrets
func (s *VaultSecretStore) SnapshotDeployment(ctx context.Context, orgID, deploymentID string, secrets map[string]string) (string, error) {
	path := s.VaultPath(orgID, ScopeDeployment, deploymentID)
	if err := s.client.PutSecret(ctx, path, secrets); err != nil {
		return "", fmt.Errorf("failed to create deployment secrets snapshot: %w", err)
	}
	return path, nil
}
