package database

import (
  "fmt"
  "gorm.io/gorm"
  "modelgate/internal/models"
)

func migrateAll(db *gorm.DB) error {
  if err := db.AutoMigrate(
    &models.User{},
    &models.APIKey{},
    &models.Model{},
    &models.BackendHealth{},
    &models.UsageStat{},
  ); err != nil {
    return fmt.Errorf("auto-migrate: %w", err)
  }
  return nil
}

func seedDefaults(db *gorm.DB) error {
  return nil
}
