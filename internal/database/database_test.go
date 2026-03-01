package database

import (
	"path/filepath"
	"testing"
)

func TestEnsureAdminKeyRequiresBootstrapKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "modelgate.db")

	if err := Init(dbPath); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = Close()
	})

	if err := EnsureAdminKey(""); err == nil {
		t.Fatalf("expected error for missing bootstrap admin key")
	}
}

func TestEnsureAdminKeyCreatesAndUpdates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "modelgate.db")

	if err := Init(dbPath); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = Close()
	})

	initialKey := "admin-initial-key"
	if err := EnsureAdminKey(initialKey); err != nil {
		t.Fatalf("create admin key: %v", err)
	}

	var createdCount int64
	if err := DB.Table("api_keys").Where("admin = ?", true).Count(&createdCount).Error; err != nil {
		t.Fatalf("count admin keys: %v", err)
	}
	if createdCount != 1 {
		t.Fatalf("expected 1 admin key, got %d", createdCount)
	}

	updatedKey := "admin-updated-key"
	if err := EnsureAdminKey(updatedKey); err != nil {
		t.Fatalf("update admin key: %v", err)
	}

	var stored struct {
		KeyHash string
	}
	if err := DB.Table("api_keys").Select("key_hash").Where("admin = ?", true).First(&stored).Error; err != nil {
		t.Fatalf("query updated admin key: %v", err)
	}

	if stored.KeyHash != hashAPIKey(updatedKey) {
		t.Fatalf("expected key hash to update")
	}
}
