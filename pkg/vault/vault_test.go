package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimulatedSecretStore(t *testing.T) {
	ctx := context.Background()
	store := NewSimulatedSecretStore()

	orgID := "org-123"
	serviceID := "svc-456"

	// 1. Put secrets
	err := store.PutSecrets(ctx, orgID, ScopeService, serviceID, map[string]string{
		"STRIPE_KEY": "sk_test_123",
		"DB_PASS":    "secret_pass",
	})
	if err != nil {
		t.Fatalf("unexpected error putting secrets: %v", err)
	}

	// 2. Get single secret
	val, err := store.GetSecretKey(ctx, orgID, ScopeService, serviceID, "STRIPE_KEY")
	if err != nil {
		t.Fatalf("unexpected error getting secret key: %v", err)
	}
	if val != "sk_test_123" {
		t.Errorf("expected sk_test_123, got %s", val)
	}

	// 3. Get all secrets
	all, err := store.GetSecrets(ctx, orgID, ScopeService, serviceID)
	if err != nil {
		t.Fatalf("unexpected error getting all secrets: %v", err)
	}
	if len(all) != 2 || all["DB_PASS"] != "secret_pass" {
		t.Errorf("expected 2 secrets, got %v", all)
	}

	// 4. Delete one key
	err = store.DeleteSecretKey(ctx, orgID, ScopeService, serviceID, "STRIPE_KEY")
	if err != nil {
		t.Fatalf("unexpected error deleting secret key: %v", err)
	}
	_, err = store.GetSecretKey(ctx, orgID, ScopeService, serviceID, "STRIPE_KEY")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}

	// 5. Snapshot deployment
	deployID := "dep-789"
	snapPath, err := store.SnapshotDeployment(ctx, orgID, deployID, map[string]string{
		"SNAPSHOT_KEY": "snapshot_val",
	})
	if err != nil {
		t.Fatalf("unexpected error snapshotting deployment: %v", err)
	}
	if snapPath != "adroit/org-123/deployments/dep-789" {
		t.Errorf("unexpected snapshot path: %s", snapPath)
	}
	snapVal, err := store.GetSecretKey(ctx, orgID, ScopeDeployment, deployID, "SNAPSHOT_KEY")
	if err != nil || snapVal != "snapshot_val" {
		t.Errorf("failed to retrieve snapshotted key: %v, val: %s", err, snapVal)
	}
}

func TestVaultHTTPClientMockServer(t *testing.T) {
	ctx := context.Background()

	var storedData map[string]string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/health" {
			w.WriteHeader(http.StatusOK)
			return
		}

		token := r.Header.Get("X-Vault-Token")
		if token != "test-token" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		switch r.Method {
		case http.MethodPost:
			var body KVv2WritePayload
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			storedData = body.Data
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"created_time":"now"}}`))

		case http.MethodGet:
			if r.URL.Path == "/v1/sys/health" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if storedData == nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			resp := KVv2ReadResponse{}
			resp.Data.Data = storedData
			resp.Data.Metadata.Version = 1
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case http.MethodDelete:
			storedData = nil
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	client := NewVaultHTTPClient(VaultConfig{
		Address: ts.URL,
		Token:   "test-token",
		Mount:   "secret",
	})

	// Health check
	if err := client.Health(ctx); err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	// Put
	err := client.PutSecret(ctx, "adroit/org-1/services/svc-1", map[string]string{
		"SECRET_TOKEN": "shhh",
	})
	if err != nil {
		t.Fatalf("PutSecret failed: %v", err)
	}

	// Get
	data, err := client.GetSecret(ctx, "adroit/org-1/services/svc-1")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if data["SECRET_TOKEN"] != "shhh" {
		t.Errorf("expected shhh, got %s", data["SECRET_TOKEN"])
	}

	// Delete
	err = client.DeleteSecret(ctx, "adroit/org-1/services/svc-1")
	if err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	// Verify deleted
	_, err = client.GetSecret(ctx, "adroit/org-1/services/svc-1")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}
