package models

import "time"

type BaseModel struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Status    string
}

type User struct {
	BaseModel
	Name  string
	Email string `gorm:"uniqueIndex"`
}

type APIKey struct {
	BaseModel
	UserID       int    `gorm:"not null;index"`
	KeyHash      string `gorm:"not null;uniqueIndex"`
	QuotaTokens  int    `gorm:"default:0"`
	UsedTokens   int    `gorm:"default:0"`
	RateLimitRPM int    `gorm:"default:60"`
}

type Model struct {
	BaseModel
	Name             string `gorm:"not null;index"`
	BackendType      string `gorm:"not null"`
	BackendURL       string `gorm:"not null"`
	BackendModelName string `gorm:"not null"`
	IsActive         bool   `gorm:"default:true"`
}

type BackendHealth struct {
	BaseModel
	BackendType   string `gorm:"not null;uniqueIndex"`
	BackendURL    string `gorm:"not null"`
	LastCheck     time.Time
	IsValid       bool
	LastError     string
	ResponseTime  int64
}

type UsageStat struct {
	ID            uint      `gorm:"primarykey"`
	Timestamp     time.Time
	APIKeyID      int
	UsedTokens    int
	Requests      int
	ResponseTime  int64
	Status        int
}
