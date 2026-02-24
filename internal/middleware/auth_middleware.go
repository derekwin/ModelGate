package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"os"

	"github.com/gin-gonic/gin"
	service "modelgate/internal/service"
	"modelgate/internal/database"
	"modelgate/internal/models"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Testing helper: allow disabling authentication via env var
		if os.Getenv("MG_AUTH_DISABLE") == "1" {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, &service.APIError{
				Message: "Invalid API Key",
				Type:    "authentication_error",
				Code:    401,
			})
			c.Abort()
			return
		}
		if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			c.JSON(http.StatusUnauthorized, &service.APIError{
				Message: "Invalid API Key",
				Type:    "authentication_error",
				Code:    401,
			})
			c.Abort()
			return
		}

		token := strings.TrimSpace(auth[len("Bearer "):])
		sum := sha256.Sum256([]byte(token))
		hashStr := hex.EncodeToString(sum[:])

		var apiKey models.APIKey
		if err := database.DB.Where("key_hash = ?", hashStr).First(&apiKey).Error; err != nil {
			c.JSON(http.StatusUnauthorized, &service.APIError{
				Message: "Invalid API Key",
				Type:    "authentication_error",
				Code:    401,
			})
			c.Abort()
			return
		}

		info := &apiKeyInfo{
			ID:           int(apiKey.ID),
			UserID:       apiKey.UserID,
			KeyHash:      apiKey.KeyHash,
			QuotaTokens:  apiKey.QuotaTokens,
			UsedTokens:   apiKey.UsedTokens,
			RateLimitRPM: apiKey.RateLimitRPM,
		}

		ctx := context.WithValue(c.Request.Context(), apiKeyContextKeyValue, info)
		c.Request = c.Request.WithContext(ctx)
		c.Set("apikey", info)

		c.Next()
	}
}
