package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

func resetConfigState() {
	cfg = nil
	once = sync.Once{}
	viper.Reset()
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func TestLoadSupportsNestedEnvOverrides(t *testing.T) {
	resetConfigState()
	t.Cleanup(resetConfigState)

	path := writeConfigFile(t, `
server:
  host: 127.0.0.1
  port: 18080
  mode: release
admin:
  api_key: bootstrap-key
`)

	t.Setenv("MG_ADMIN_API_KEY", "override-key")
	t.Setenv("MG_SERVER_PORT", "118080")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.Admin.APIKey != "override-key" {
		t.Fatalf("expected admin api key override, got %q", loaded.Admin.APIKey)
	}
	if loaded.Server.Port != 118080 {
		t.Fatalf("expected server port override 118080, got %d", loaded.Server.Port)
	}
}

func TestReloadRefreshesInMemoryConfig(t *testing.T) {
	resetConfigState()
	t.Cleanup(resetConfigState)

	path := writeConfigFile(t, `
server:
  host: 127.0.0.1
  port: 18080
  mode: release
admin:
  api_key: bootstrap-key
`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Admin.APIKey != "bootstrap-key" {
		t.Fatalf("expected initial admin api key bootstrap-key, got %q", loaded.Admin.APIKey)
	}

	if err := os.WriteFile(path, []byte(`
server:
  host: 127.0.0.1
  port: 8088
  mode: release
admin:
  api_key: updated-key
`), 0o644); err != nil {
		t.Fatalf("rewrite config file: %v", err)
	}

	if err := Reload(); err != nil {
		t.Fatalf("reload config: %v", err)
	}

	reloaded := Get()
	if reloaded.Server.Port != 8088 {
		t.Fatalf("expected reloaded server port 8088, got %d", reloaded.Server.Port)
	}
	if reloaded.Admin.APIKey != "updated-key" {
		t.Fatalf("expected reloaded admin api key updated-key, got %q", reloaded.Admin.APIKey)
	}
}
