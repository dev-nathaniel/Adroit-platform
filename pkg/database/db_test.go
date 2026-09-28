package database_test

import (
	"os"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
)

func TestLoadConfig(t *testing.T) {
	os.Setenv("DB_HOST", "postgres.internal")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "custom_user")
	os.Setenv("DB_PASSWORD", "custom_pass")
	os.Setenv("DB_NAME", "custom_db")
	os.Setenv("DB_SSLMODE", "require")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_SSLMODE")
	}()

	cfg := database.LoadConfig()

	if cfg.Host != "postgres.internal" {
		t.Errorf("expected host postgres.internal, got %s", cfg.Host)
	}
	if cfg.Port != 5433 {
		t.Errorf("expected port 5433, got %d", cfg.Port)
	}

	expectedDSN := "postgres://custom_user:custom_pass@postgres.internal:5433/custom_db?sslmode=require"
	if cfg.DSN() != expectedDSN {
		t.Errorf("expected DSN '%s', got '%s'", expectedDSN, cfg.DSN())
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := database.Config{
		Host:            "localhost",
		Port:            5432,
		User:            "adroit",
		Password:        "adroit_secret",
		Database:        "adroit_platform",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
	}

	expectedDSN := "postgres://adroit:adroit_secret@localhost:5432/adroit_platform?sslmode=disable"
	if cfg.DSN() != expectedDSN {
		t.Errorf("expected DSN %s, got %s", expectedDSN, cfg.DSN())
	}
}
