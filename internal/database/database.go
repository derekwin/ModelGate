package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"modelgate/internal/models"
)

var DB *gorm.DB

func Init(dbPath string) error {
	var err error

	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	err = DB.AutoMigrate(
		&models.APIKey{},
		&models.Model{},
		&models.UsageRecord{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func EnsureAdminKey(adminAPIKey string) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	var adminKey models.APIKey
	result := DB.Where("admin = ?", true).First(&adminKey)

	if result.Error == nil {
		if strings.TrimSpace(adminAPIKey) == "" {
			return nil
		}

		keyHash := hashAPIKey(adminAPIKey)
		if adminKey.KeyHash == keyHash {
			return nil
		}

		log.Printf("Updating admin API key in database\n")
		adminKey.Key = adminAPIKey
		adminKey.KeyHash = keyHash
		adminKey.Name = "admin"
		adminKey.Quota = 100000000
		adminKey.RateLimit = 10000
		adminKey.Admin = true
		adminKey.BaseModel.Status = "active"
		if err := DB.Save(&adminKey).Error; err != nil {
			return fmt.Errorf("failed to update admin key: %w", err)
		}
		return nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to query admin key: %w", result.Error)
	}

	if strings.TrimSpace(adminAPIKey) == "" {
		return fmt.Errorf("admin api key must be configured for first startup")
	}

	keyHash := hashAPIKey(adminAPIKey)

	newKey := models.APIKey{
		Key:       adminAPIKey,
		KeyHash:   keyHash,
		Name:      "admin",
		Quota:     100000000,
		RateLimit: 10000,
		Admin:     true,
	}
	newKey.BaseModel.Status = "active"

	if err := DB.Create(&newKey).Error; err != nil {
		return fmt.Errorf("failed to create admin key: %w", err)
	}

	return nil
}

func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
