package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"modelgate/internal/database"
	"modelgate/internal/models"
)

func HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

func GenerateAPIKey() string {
	return generateRandomKey(32)
}

func generateRandomKey(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			num = big.NewInt(int64(i % len(letters)))
		}
		result[i] = letters[num.Int64()]
	}
	return string(result)
}

func ValidateAPIKey(apiKey string) (*models.APIKey, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("api key is empty")
	}

	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	keyHash := HashAPIKey(apiKey)

	var apiKeyModel models.APIKey
	result := db.Where("key_hash = ? AND status = ?", keyHash, "active").First(&apiKeyModel)
	if result.Error != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	return &apiKeyModel, nil
}

func CheckIPAllowed(apiKey *models.APIKey, ip string) bool {
	if apiKey == nil {
		return false
	}
	if apiKey.AllowedIPs == "" {
		return true
	}

	allowedIPs := splitIPs(apiKey.AllowedIPs)
	for _, allowed := range allowedIPs {
		if allowed == ip {
			return true
		}
	}
	return false
}

func splitIPs(ips string) []string {
	var result []string
	var current string
	for _, c := range ips {
		if c == ',' {
			if current != "" {
				result = append(result, trimSpace(current))
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, trimSpace(current))
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
