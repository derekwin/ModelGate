package models

import (
	"time"
)

type BaseModel struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `gorm:"default:active" json:"status"`
}

type APIKey struct {
	BaseModel
	Key          string `gorm:"uniqueIndex;not null" json:"-"`
	KeyHash      string `gorm:"uniqueIndex;not null" json:"key_hash"`
	Name         string `json:"name"`
	Quota        int64  `gorm:"default:1000000" json:"quota"`
	QuotaUsed    int64  `gorm:"default:0" json:"quota_used"`
	RateLimit    int    `gorm:"default:60" json:"rate_limit"`
	AllowedIPs   string `json:"allowed_ips"`
	Admin        bool   `gorm:"default:false" json:"admin"`
	Tier         string `gorm:"default:free" json:"tier"` // free, paid
	DefaultModel string `json:"default_model"`            // model name for this tier
}

type Model struct {
	BaseModel
	Name        string `gorm:"uniqueIndex;not null" json:"name"`
	BackendType string `gorm:"not null" json:"backend_type"`
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"-"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`
}

type UsageRecord struct {
	ID               uint      `gorm:"primarykey" json:"id"`
	APIKeyID         uint      `gorm:"index;not null" json:"api_key_id"`
	Model            string    `gorm:"not null" json:"model"`
	PromptTokens     int64     `gorm:"not null" json:"prompt_tokens"`
	CompletionTokens int64     `gorm:"not null" json:"completion_tokens"`
	TotalTokens      int64     `gorm:"not null" json:"total_tokens"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
}
