package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
)

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_KEY_STRING", "production")
	defer os.Unsetenv("TEST_KEY_STRING")

	if val := config.GetEnv("TEST_KEY_STRING", "dev"); val != "production" {
		t.Errorf("expected 'production', got '%s'", val)
	}

	if val := config.GetEnv("NON_EXISTENT_KEY", "fallback"); val != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", val)
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_PORT", "9000")
	defer os.Unsetenv("TEST_PORT")

	if val := config.GetEnvInt("TEST_PORT", 8080); val != 9000 {
		t.Errorf("expected 9000, got %d", val)
	}

	if val := config.GetEnvInt("NON_EXISTENT_PORT", 8080); val != 8080 {
		t.Errorf("expected 8080, got %d", val)
	}
}

func TestGetEnvDuration(t *testing.T) {
	os.Setenv("TEST_TIMEOUT", "30s")
	defer os.Unsetenv("TEST_TIMEOUT")

	if val := config.GetEnvDuration("TEST_TIMEOUT", 5*time.Second); val != 30*time.Second {
		t.Errorf("expected 30s, got %v", val)
	}
}
