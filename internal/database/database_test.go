package database

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestEnsureAdminKeyRequiresBootstrapKey(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := NewGormFromSQLDB(sqlDB)
	if err != nil {
		t.Fatalf("gorm init: %v", err)
	}
	restore := SetDBForTesting(gormDB)
	t.Cleanup(func() {
		restore()
		_ = sqlDB.Close()
	})

	mock.ExpectQuery(`SELECT \* FROM "api_keys" WHERE admin = \$1 ORDER BY "api_keys"\."id" LIMIT \$2`).
		WithArgs(true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if err := EnsureAdminKey(""); err == nil {
		t.Fatalf("expected error for missing bootstrap admin key")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEnsureAdminKeyCreateAndUpdatePaths(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := NewGormFromSQLDB(sqlDB)
	if err != nil {
		t.Fatalf("gorm init: %v", err)
	}
	restore := SetDBForTesting(gormDB)
	t.Cleanup(func() {
		restore()
		_ = sqlDB.Close()
	})

	initialKey := "admin-initial-key"
	mock.ExpectQuery(`SELECT \* FROM "api_keys" WHERE admin = \$1 ORDER BY "api_keys"\."id" LIMIT \$2`).
		WithArgs(true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`INSERT INTO "api_keys"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	if err := EnsureAdminKey(initialKey); err != nil {
		t.Fatalf("create admin key: %v", err)
	}

	updatedKey := "admin-updated-key"
	mock.ExpectQuery(`SELECT \* FROM "api_keys" WHERE admin = \$1 ORDER BY "api_keys"\."id" LIMIT \$2`).
		WithArgs(true, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "status", "key", "key_hash", "name", "quota", "quota_used", "rate_limit", "allowed_ips", "admin", "tier", "default_model",
		}).AddRow(
			1, time.Now(), time.Now(), "active", initialKey, hashAPIKey(initialKey), "admin", 100000000, 0, 10000, "", true, "free", "",
		))
	mock.ExpectExec(`UPDATE "api_keys" SET .* WHERE "id" = \$14`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := EnsureAdminKey(updatedKey); err != nil {
		t.Fatalf("update admin key: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEnsureAdminKeyUnexpectedQueryError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := NewGormFromSQLDB(sqlDB)
	if err != nil {
		t.Fatalf("gorm init: %v", err)
	}
	restore := SetDBForTesting(gormDB)
	t.Cleanup(func() {
		restore()
		_ = sqlDB.Close()
	})

	mock.ExpectQuery(`SELECT \* FROM "api_keys" WHERE admin = \$1 ORDER BY "api_keys"\."id" LIMIT \$2`).
		WithArgs(true, 1).
		WillReturnError(gorm.ErrInvalidDB)

	if err := EnsureAdminKey("admin-key"); err == nil {
		t.Fatalf("expected query error")
	}
}
