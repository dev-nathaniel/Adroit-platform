package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_LoadSaveAndActiveContext(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ADROIT_CONFIG_DIR", tempDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading non-existent config: %v", err)
	}

	if cfg.CurrentContext != DefaultContextName {
		t.Errorf("expected current context %q, got %q", DefaultContextName, cfg.CurrentContext)
	}

	active := cfg.ActiveContext()
	if active.APIURL != DefaultLocalAPIURL {
		t.Errorf("expected api url %q, got %q", DefaultLocalAPIURL, active.APIURL)
	}

	// Update active context
	active.Token = "test_token_123"
	active.ActiveOrgID = "org_abc"
	active.ActiveOrgName = "Test Org"
	active.UserEmail = "test@adroit.run"

	if err := Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Reload config
	reloaded, err := Load()
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	reloadedActive := reloaded.ActiveContext()
	if reloadedActive.Token != "test_token_123" {
		t.Errorf("expected token %q, got %q", "test_token_123", reloadedActive.Token)
	}
	if reloadedActive.ActiveOrgID != "org_abc" {
		t.Errorf("expected org id %q, got %q", "org_abc", reloadedActive.ActiveOrgID)
	}
}

func TestProjectConfig_FindAndSave(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "pkg", "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	pcfg := DefaultProjectConfig("my-api", 8080)
	pcfg.Service.ID = "svc_123"
	projectFile := filepath.Join(tempDir, ProjectFileName)

	if err := SaveProjectConfig(projectFile, pcfg); err != nil {
		t.Fatalf("failed to save project config: %v", err)
	}

	// Find from deep sub-directory
	found, foundPath, err := FindProjectConfig(subDir)
	if err != nil {
		t.Fatalf("failed to find project config from subdir: %v", err)
	}

	if foundPath != projectFile {
		t.Errorf("expected found path %q, got %q", projectFile, foundPath)
	}
	if found.Service.Name != "my-api" {
		t.Errorf("expected service name %q, got %q", "my-api", found.Service.Name)
	}
	if found.Service.ID != "svc_123" {
		t.Errorf("expected service id %q, got %q", "svc_123", found.Service.ID)
	}
}
