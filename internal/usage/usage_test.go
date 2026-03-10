package usage

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"modelgate/internal/database"
)

func TestUpdateAPIKeyQuotaAtomicGuard(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := database.NewGormFromSQLDB(sqlDB)
	if err != nil {
		t.Fatalf("gorm init: %v", err)
	}
	restore := database.SetDBForTesting(gormDB)
	t.Cleanup(func() {
		restore()
		_ = sqlDB.Close()
	})

	mock.ExpectExec(`UPDATE "api_keys" SET "quota_used"=\(quota_used \+ \$1\) WHERE id = \$2 AND quota_used \+ \$3 <= quota`).
		WithArgs(int64(3), uint(1), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err = UpdateAPIKeyQuota(1, 3)
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("expected ErrInsufficientQuota, got %v", err)
	}

	mock.ExpectExec(`UPDATE "api_keys" SET "quota_used"=\(quota_used \+ \$1\) WHERE id = \$2 AND quota_used \+ \$3 <= quota`).
		WithArgs(int64(2), uint(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := UpdateAPIKeyQuota(1, 2); err != nil {
		t.Fatalf("expected successful update, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
