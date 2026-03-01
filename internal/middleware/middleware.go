package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"modelgate/internal/auth"
	"modelgate/internal/limiter"
	"modelgate/internal/models"
)

type AuthMiddleware struct {
	limiter *limiter.RateLimiter
}

func NewAuthMiddleware(limiter *limiter.RateLimiter) *AuthMiddleware {
	return &AuthMiddleware{
		limiter: limiter,
	}
}

func (m *AuthMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing authorization header",
					"type":    "invalid_request_error",
					"code":    401,
				},
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid authorization header format",
					"type":    "invalid_request_error",
					"code":    401,
				},
			})
			c.Abort()
			return
		}

		apiKey := parts[1]
		apiKeyModel, err := auth.ValidateAPIKey(apiKey)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
					"code":    401,
				},
			})
			c.Abort()
			return
		}

		clientIP := c.ClientIP()
		if !auth.CheckIPAllowed(apiKeyModel, clientIP) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": "IP not allowed",
					"type":    "invalid_request_error",
					"code":    403,
				},
			})
			c.Abort()
			return
		}

		c.Set("api_key", apiKey)
		c.Set("api_key_model", apiKeyModel)
		c.Set("user_id", apiKeyModel.ID)

		c.Next()
	}
}

func (m *AuthMiddleware) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.limiter == nil {
			c.Next()
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			c.Next()
			return
		}

		key := apiKeyModel.(*models.APIKey)
		keyHash := key.KeyHash
		limit := m.limiter.EffectiveLimit(key.RateLimit)
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))

		allowed, err := m.limiter.Allow(keyHash, key.RateLimit)
		if err != nil {
			log.Error().Err(err).Msg("Rate limiter error")
			c.Next()
			return
		}

		if !allowed {
			remaining, _ := m.limiter.GetRemaining(keyHash, key.RateLimit)
			c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
					"code":    429,
				},
			})
			c.Abort()
			return
		}

		remaining, _ := m.limiter.GetRemaining(keyHash, key.RateLimit)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()
		if path == "/health" {
			return
		}

		var userID int
		if uid, exists := c.Get("user_id"); exists {
			switch v := uid.(type) {
			case int:
				userID = v
			case uint:
				userID = int(v)
			case int64:
				userID = int(v)
			case uint64:
				userID = int(v)
			}
		}

		event := log.Debug()
		switch {
		case statusCode >= http.StatusInternalServerError:
			event = log.Error()
		case statusCode >= http.StatusBadRequest:
			event = log.Warn()
		}

		event.
			Str("method", method).
			Str("path", path).
			Str("ip", clientIP).
			Int("user_id", userID).
			Int("status", statusCode).
			Dur("latency", latency).
			Msg("Request processed")
	}
}

func BodySizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"message": "Request body too large",
					"type":    "invalid_request_error",
					"code":    400,
				},
			})
			c.Abort()
			return
		}

		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		}

		c.Next()
	}
}
