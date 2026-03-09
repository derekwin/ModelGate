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
  port: 8080
  mode: release
admin:
  api_key: bootstrap-key
  host: 127.0.0.1
  port: 8081
`)

	t.Setenv("MG_ADMIN_PORT", "9090")
	t.Setenv("MG_SERVER_PORT", "18080")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if loaded.Admin.Port != 9090 {
		t.Fatalf("expected admin port override 9090, got %d", loaded.Admin.Port)
	}
	if loaded.Server.Port != 18080 {
		t.Fatalf("expected server port override 18080, got %d", loaded.Server.Port)
	}
}

func TestReloadRefreshesInMemoryConfig(t *testing.T) {
	resetConfigState()
	t.Cleanup(resetConfigState)

	path := writeConfigFile(t, `
server:
  host: 127.0.0.1
  port: 8080
  mode: release
admin:
  api_key: bootstrap-key
  host: 127.0.0.1
  port: 8081
`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Admin.Port != 8081 {
		t.Fatalf("expected initial admin port 8081, got %d", loaded.Admin.Port)
	}

	if err := os.WriteFile(path, []byte(`
server:
  host: 127.0.0.1
  port: 8088
  mode: release
admin:
  api_key: bootstrap-key
  host: 127.0.0.1
  port: 9091
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
	if reloaded.Admin.Port != 9091 {
		t.Fatalf("expected reloaded admin port 9091, got %d", reloaded.Admin.Port)
	}
}
