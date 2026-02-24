package middleware

import (
    "net/http"

    "github.com/gin-gonic/gin"
    service "modelgate/internal/service"
)

// QuotaMiddleware checks that the API key has not exceeded its token quota.
// If the quota is exceeded, a 402 (Payment Required) is returned.
func QuotaMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        var info *apiKeyInfo
        if v, ok := c.Get("apikey"); ok {
            if i, ok2 := v.(*apiKeyInfo); ok2 {
                info = i
            }
        }
        if info == nil {
            if v := c.Request.Context().Value(apiKeyContextKeyValue); v != nil {
                if i, ok := v.(*apiKeyInfo); ok {
                    info = i
                }
            }
        }
        if info == nil {
            c.JSON(http.StatusUnauthorized, &service.APIError{
                Message: "Invalid API Key",
                Type:    "authentication_error",
                Code:    401,
            })
            c.Abort()
            return
        }

        // If quota is defined, enforce it.
        if info.QuotaTokens > 0 {
            if info.UsedTokens >= info.QuotaTokens {
                c.JSON(http.StatusPaymentRequired, &service.APIError{
                    Message: "Quota exceeded",
                    Type:    "quota_error",
                    Code:    402,
                })
                c.Abort()
                return
            }
        }

        // Note: Token usage accounting should be incremented by the usage subsystem
        // as part of a transactional operation. This middleware only enforces the check.
        c.Next()
    }
}
