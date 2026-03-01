package usage

import (
	"errors"
	"path/filepath"
	"testing"

	"modelgate/internal/database"
	"modelgate/internal/models"
)

func TestUpdateAPIKeyQuotaAtomicGuard(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "modelgate.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	key := models.APIKey{
		Key:       "test-key",
		KeyHash:   "test-key-hash",
		Name:      "test",
		Quota:     10,
		QuotaUsed: 8,
	}
	key.BaseModel.Status = "active"
	if err := database.GetDB().Create(&key).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}

	err := UpdateAPIKeyQuota(key.ID, 3)
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("expected ErrInsufficientQuota, got %v", err)
	}

	var afterReject models.APIKey
	if err := database.GetDB().First(&afterReject, key.ID).Error; err != nil {
		t.Fatalf("query key after reject: %v", err)
	}
	if afterReject.QuotaUsed != 8 {
		t.Fatalf("expected quota_used to stay 8, got %d", afterReject.QuotaUsed)
	}

	if err := UpdateAPIKeyQuota(key.ID, 2); err != nil {
		t.Fatalf("expected successful update, got %v", err)
	}

	var afterSuccess models.APIKey
	if err := database.GetDB().First(&afterSuccess, key.ID).Error; err != nil {
		t.Fatalf("query key after success: %v", err)
	}
	if afterSuccess.QuotaUsed != 10 {
		t.Fatalf("expected quota_used to be 10, got %d", afterSuccess.QuotaUsed)
	}
}
