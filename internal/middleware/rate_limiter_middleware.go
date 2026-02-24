package middleware

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/go-redis/redis/v8"
    service "modelgate/internal/service"
)

// redisClient is a lightweight shared client for rate limiting. In a full
// application this would be provided by the DI/container. Here we lazily
// initialize a singleton client using environment-configured address.
var redisClient *redis.Client

func getRedisClient() *redis.Client {
    if redisClient != nil {
        return redisClient
    }
    addr := os.Getenv("REDIS_ADDR")
    if addr == "" {
        addr = "localhost:6379"
    }
    redisClient = redis.NewClient(&redis.Options{Addr: addr})
    return redisClient
}

// RateLimiterMiddleware enforces per-api-key rate limits per minute window.
// It uses Redis INCR with an auto-expiring key.
func RateLimiterMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Retrieve API key info from context
        var info *apiKeyInfo
        if v, ok := c.Get("apikey"); ok {
            if i, ok2 := v.(*apiKeyInfo); ok2 {
                info = i
            }
        }
        if info == nil {
            // Try from request context as fallback
            if v := c.Request.Context().Value(apiKeyContextKeyValue); v != nil {
                if i, ok := v.(*apiKeyInfo); ok {
                    info = i
                }
            }
        }
        if info == nil {
            // No API key information available; deny access
            c.JSON(http.StatusUnauthorized, &service.APIError{
                Message: "Invalid API Key",
                Type:    "authentication_error",
                Code:    401,
            })
            c.Abort()
            return
        }

        // Build Redis key: ratelimit:{hash}:{minute}
        minute := time.Now().UTC().Unix() / 60
        key := fmt.Sprintf("ratelimit:%s:%d", info.KeyHash, minute)

        client := getRedisClient()
        ctx := context.Background()
        count, err := client.Incr(ctx, key).Result()
        if err != nil {
            // If Redis is unavailable, fail open to avoid blocking legitimate users
            c.Next()
            return
        }
        if count == 1 {
            // Set expiry for the minute window
            _ = client.Expire(ctx, key, time.Minute)
        }
        if info.RateLimitRPM > 0 && int(count) > info.RateLimitRPM {
            c.JSON(http.StatusTooManyRequests, &service.APIError{
                Message: "Rate limit exceeded",
                Type:    "rate_limit_error",
                Code:    429,
            })
            c.Abort()
            return
        }

        c.Next()
    }
}
