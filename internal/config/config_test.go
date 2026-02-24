package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
server:
  port: 8080
  max_body_mb: 5
  timeout_seconds: 300

database:
  path: ./data/modelgate.db

redis:
  addr: localhost:6379
  password: ""
  db: 0
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	if err := LoadConfig(configPath); err != nil {
		t.Errorf("LoadConfig() error = %v", err)
	}

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Server.MaxBodyMB != 5 {
		t.Errorf("Expected max_body_mb 5, got %d", cfg.Server.MaxBodyMB)
	}

	if cfg.Server.TimeoutSeconds != 300 {
		t.Errorf("Expected timeout_seconds 300, got %d", cfg.Server.TimeoutSeconds)
	}
}
