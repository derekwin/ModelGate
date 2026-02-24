package middleware

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"
)

func RequestLogger() gin.HandlerFunc {
    return func(c *gin.Context) {
        t1 := time.Now()
        c.Next()
        latency := time.Since(t1)
        log.Info().Str("method", c.Request.Method).Str("path", c.Request.URL.Path).
            Dur("latency", latency).Int("status", c.Writer.Status()).Msg("request")
    }
}

func logToContext() gin.HandlerFunc {
    return func(c *gin.Context) {
        logger := log.Ctx(c.Request.Context()).With().
            Str("request_id", c.GetString("X-Request-ID")).
            Logger()
        c.Set("logger", logger)
        c.Request = c.Request.WithContext(logger.WithContext(c.Request.Context()))
        c.Next()
    }
}

func BodySizeLimiter(maxBytes int) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxBytes)*1024*1024)
        c.Next()
    }
}
