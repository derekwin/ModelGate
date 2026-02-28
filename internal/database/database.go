package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
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
	var adminKey models.APIKey
	result := DB.Where("admin = ?", true).First(&adminKey)

	if adminAPIKey == "" {
		if result.Error == nil {
			return nil
		}
		apiKey := "mg_admin_" + generateRandomKey(24)
		log.Printf("No admin API key configured, auto-generated: %s\n", apiKey)
		log.Printf("Please save this key - it will not be shown again!\n")
		adminAPIKey = apiKey
	} else {
		log.Printf("Using configured admin API key from config\n")
	}

	keyHash := hashAPIKey(adminAPIKey)

	if result.Error == nil {
		if adminKey.KeyHash != keyHash {
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
		}
		return nil
	}

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

func generateRandomKey(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	for i := range result {
		result[i] = letters[i%len(letters)]
	}
	return string(result)
}

func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
