package database

import (
  "context"
  "fmt"
  "time"

  "gorm.io/driver/sqlite"
  "gorm.io/gorm"
  "github.com/rs/zerolog/log"
)

var (
  // DB holds the global database handle
  DB *gorm.DB
)

// GetDB returns the underlying *gorm.DB instance.
func GetDB() *gorm.DB { return DB }

// InitDatabase initializes the SQLite database and runs migrations.
func InitDatabase(path string) (*gorm.DB, error) {
  if DB != nil {
    return DB, nil
  }

  db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
  if err != nil {
    return nil, fmt.Errorf("database: open sqlite: %w", err)
  }
  DB = db

  // Basic health check after connect
  sqlDB, err := db.DB()
  if err != nil {
    return nil, fmt.Errorf("database: access sqlDB: %w", err)
  }
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  if err := sqlDB.PingContext(ctx); err != nil {
    return nil, fmt.Errorf("database: ping: %w", err)
  }
  log.Info().Str("path", path).Msg("database: connected")

  // Run migrations and seeds (no-ops if none defined yet)
  if err := migrateAll(db); err != nil {
    return nil, fmt.Errorf("database: migrate: %w", err)
  }
  if err := seedDefaults(db); err != nil {
    return nil, fmt.Errorf("database: seed: %w", err)
  }

  return db, nil
}

// CloseDatabase gracefully closes the database connection.
func CloseDatabase() error {
  if DB == nil {
    return nil
  }
  sqlDB, err := DB.DB()
  if err != nil {
    return fmt.Errorf("database: get sql db: %w", err)
  }
  if err := sqlDB.Close(); err != nil {
    return fmt.Errorf("database: close: %w", err)
  }
  log.Info().Msg("database: closed")
  DB = nil
  return nil
}

// HealthCheck verifies the database is reachable.
func HealthCheck() error {
  if DB == nil {
    return fmt.Errorf("database: not initialized")
  }
  sqlDB, err := DB.DB()
  if err != nil {
    return fmt.Errorf("database: get sql db: %w", err)
  }
  ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
  defer cancel()
  if err := sqlDB.PingContext(ctx); err != nil {
    return fmt.Errorf("database: ping: %w", err)
  }
  return nil
}
