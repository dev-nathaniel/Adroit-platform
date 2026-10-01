package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrSecretNotFound = errors.New("secret not found in vault")
	ErrVaultDisabled  = errors.New("vault client is disabled or unconfigured")
)

// VaultConfig defines connection parameters for HashiCorp Vault
type VaultConfig struct {
	Address   string        `json:"address"`
	Token     string        `json:"token"`
	Mount     string        `json:"mount"`     // e.g. "secret"
	Namespace string        `json:"namespace"` // optional Enterprise namespace
	Timeout   time.Duration `json:"timeout"`
}

// VaultHTTPClient communicates directly with the HashiCorp Vault HTTP REST API (KV v2)
type VaultHTTPClient struct {
	address    string
	token      string
	mount      string
	namespace  string
	httpClient *http.Client
}

// NewVaultHTTPClient creates an HTTP client for Vault KV v2 engine
func NewVaultHTTPClient(cfg VaultConfig) *VaultHTTPClient {
	addr := strings.TrimRight(cfg.Address, "/")
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}
	mount := strings.Trim(cfg.Mount, "/")
	if mount == "" {
		mount = "secret"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &VaultHTTPClient{
		address:   addr,
		token:     cfg.Token,
		mount:     mount,
		namespace: cfg.Namespace,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// KVv2WritePayload represents the body sent to Vault KV v2
type KVv2WritePayload struct {
	Data map[string]string `json:"data"`
}

// KVv2ReadResponse represents the response envelope from Vault KV v2
type KVv2ReadResponse struct {
	Data struct {
		Data     map[string]string `json:"data"`
		Metadata struct {
			Version int `json:"version"`
		} `json:"metadata"`
	} `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

// PutSecret writes key-value pairs into Vault KV v2 at {mount}/data/{path}
func (c *VaultHTTPClient) PutSecret(ctx context.Context, path string, data map[string]string) error {
	cleanPath := strings.TrimPrefix(path, "/")
	url := fmt.Sprintf("%s/v1/%s/data/%s", c.address, c.mount, cleanPath)

	bodyBytes, err := json.Marshal(KVv2WritePayload{Data: data})
	if err != nil {
		return fmt.Errorf("failed to marshal secret payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create vault request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vault put request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetSecret reads key-value pairs from Vault KV v2 at {mount}/data/{path}
func (c *VaultHTTPClient) GetSecret(ctx context.Context, path string) (map[string]string, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	url := fmt.Sprintf("%s/v1/%s/data/%s", c.address, c.mount, cleanPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var vaultResp KVv2ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("failed to decode vault response: %w", err)
	}

	if vaultResp.Data.Data == nil {
		return make(map[string]string), nil
	}

	return vaultResp.Data.Data, nil
}

// DeleteSecret removes secret metadata and all versions at {mount}/metadata/{path}
func (c *VaultHTTPClient) DeleteSecret(ctx context.Context, path string) error {
	cleanPath := strings.TrimPrefix(path, "/")
	url := fmt.Sprintf("%s/v1/%s/metadata/%s", c.address, c.mount, cleanPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create vault delete request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vault delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault delete error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Health checks if Vault is accessible
func (c *VaultHTTPClient) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/sys/health?sealedcode=200&uninitcode=200", c.address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault unhealthy: HTTP %d", resp.StatusCode)
	}
	return nil
}
