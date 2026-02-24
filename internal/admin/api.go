package admin

import (
	"context"
	"time"
	"fmt"

	"github.com/gin-gonic/gin"
	"modelgate/internal/database"
	"modelgate/internal/models"
)

type UsageService struct{}

func NewUsageService() *UsageService {
	return &UsageService{}
}

func (us *UsageService) RecordUsage(ctx context.Context, apiKeyID int, tokens, responseTime int64, status int) error {
	stat := &models.UsageStat{
		Timestamp:     time.Now(),
		APIKeyID:      apiKeyID,
		UsedTokens:    int(tokens),
		Requests:      1,
		ResponseTime:  responseTime,
		Status:        status,
	}
	return database.DB.Create(stat).Error
}

func (us *UsageService) GetStatsByTimeRange(ctx context.Context, startTime, endTime time.Time) ([]UsageStatsByTime, error) {
	var stats []UsageStatsByTime
	query := `
		SELECT 
			date_trunc('hour', timestamp) as time_bucket,
			SUM(used_tokens) as total_tokens,
			SUM(requests) as total_requests,
			AVG(response_time) as avg_response_time,
			COUNT(*) as sample_count
		FROM usage_stats
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY time_bucket
		ORDER BY time_bucket ASC
	`

	if err := database.DB.Raw(query, startTime, endTime).Scan(&stats).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

func (us *UsageService) GetStatsByAPIKey(ctx context.Context, startTime, endTime time.Time) ([]UsageStatsByKey, error) {
	var stats []UsageStatsByKey
	query := `
		SELECT 
			api_key_id,
			SUM(used_tokens) as total_tokens,
			SUM(requests) as total_requests,
			AVG(response_time) as avg_response_time
		FROM usage_stats
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY api_key_id
		ORDER BY total_tokens DESC
	`

	if err := database.DB.Raw(query, startTime, endTime).Scan(&stats).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

func (us *UsageService) GetRecentLogs(ctx context.Context, limit int) ([]models.UsageStat, error) {
	var logs []models.UsageStat
	if err := database.DB.Order("timestamp DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

type UsageStatsByTime struct {
	TimeBucket      time.Time `json:"time_bucket"`
	TotalTokens     int64     `json:"total_tokens"`
	TotalRequests   int       `json:"total_requests"`
	AvgResponseTime int64     `json:"avg_response_time"`
	SampleCount     int       `json:"sample_count"`
}

type UsageStatsByKey struct {
	APIKeyID        int   `json:"api_key_id"`
	TotalTokens     int64 `json:"total_tokens"`
	TotalRequests   int   `json:"total_requests"`
	AvgResponseTime int64 `json:"avg_response_time"`
}

func RecordUsage(ctx context.Context, apiKeyID int, tokens, responseTime int64, status int) error {
	stat := &models.UsageStat{
		Timestamp:     time.Now(),
		APIKeyID:      apiKeyID,
		UsedTokens:    int(tokens),
		Requests:      1,
		ResponseTime:  responseTime,
		Status:        status,
	}
	return database.DB.Create(stat).Error
}

type AdminAPIHandler struct{}

func NewAdminAPIHandler() *AdminAPIHandler {
	return &AdminAPIHandler{}
}

func (h *AdminAPIHandler) RegisterRoutes(router *gin.Engine) {
	admin := router.Group("/admin")
	{
		admin.GET("/health", h.GetHealthStatus)
		admin.GET("/health/all", h.GetAllHealthStatus)
		admin.POST("/health/check", h.CheckAllBackends)

		admin.GET("/tenants/users", h.ListUsers)
		admin.POST("/tenants/users", h.CreateUser)
		admin.PUT("/tenants/users/:id", h.UpdateUser)
		admin.DELETE("/tenants/users/:id", h.DeleteUser)

		admin.GET("/tenants/apikeys", h.ListAPIKeys)
		admin.POST("/tenants/apikeys", h.CreateAPIKey)
		admin.PUT("/tenants/apikeys/:id", h.UpdateAPIKey)
		admin.DELETE("/tenants/apikeys/:id", h.DeleteAPIKey)

		admin.GET("/tenants/models", h.ListModels)
		admin.POST("/tenants/models", h.CreateModel)
		admin.PUT("/tenants/models/:id", h.UpdateModel)
		admin.DELETE("/tenants/models/:id", h.DeleteModel)

		admin.GET("/usage/stats", h.GetUsageStats)
		admin.GET("/usage/stats/by-time", h.GetUsageStatsByTime)
		admin.GET("/usage/stats/by-key", h.GetUsageStatsByKey)
		admin.GET("/usage/logs", h.GetRecentLogs)
	}
}

func (h *AdminAPIHandler) GetHealthStatus(c *gin.Context) {
	ctx := c.Request.Context()
	backendType := c.Query("backend")
	if backendType == "" {
		backendType = "all"
	}

	health, err := GetBackendHealth(ctx, backendType)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   health,
	})
}

func (h *AdminAPIHandler) GetAllHealthStatus(c *gin.Context) {
	ctx := c.Request.Context()
	healths, err := GetAllBackendHealth(ctx)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   healths,
	})
}

func (h *AdminAPIHandler) CheckAllBackends(c *gin.Context) {
	ctx := c.Request.Context()
	if err := CheckAllBackends(ctx); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"message": "Health check completed",
	})
}

func (h *AdminAPIHandler) ListUsers(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	users, err := NewTenantService().ListUsers(ctx, status)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   users,
	})
}

func (h *AdminAPIHandler) CreateUser(c *gin.Context) {
	var input struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	user, err := NewTenantService().CreateUser(ctx, input.Name, input.Email)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"status": "ok",
		"data":   user,
	})
}

func (h *AdminAPIHandler) UpdateUser(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    var input struct {
        Name   string `json:"name"`
        Email  string `json:"email"`
        Status string `json:"status"`
    }

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

    ctx := c.Request.Context()
    user, err := NewTenantService().UpdateUser(ctx, idUint, map[string]interface{}{
        "name":   input.Name,
        "email":  input.Email,
        "status": input.Status,
    })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   user,
	})
}

func (h *AdminAPIHandler) DeleteUser(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    ctx := c.Request.Context()
    if err := NewTenantService().DeleteUser(ctx, idUint); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"message": "User deleted",
	})
}

func (h *AdminAPIHandler) ListAPIKeys(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	apiKeys, err := NewTenantService().ListAPIKeys(ctx, status)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   apiKeys,
	})
}

func (h *AdminAPIHandler) CreateAPIKey(c *gin.Context) {
	var input struct {
		UserID       int `json:"user_id" binding:"required"`
		QuotaTokens  int `json:"quota_tokens" binding:"required"`
		RateLimitRPM int `json:"rate_limit_rpm" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	apiKey, err := NewTenantService().CreateAPIKey(ctx, input.UserID, input.QuotaTokens, input.RateLimitRPM)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"status": "ok",
		"data":   apiKey,
	})
}

func (h *AdminAPIHandler) UpdateAPIKey(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
	var input struct {
		QuotaTokens  int `json:"quota_tokens"`
		RateLimitRPM int `json:"rate_limit_rpm"`
		Status       string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

    ctx := c.Request.Context()
    apiKey, err := NewTenantService().UpdateAPIKey(ctx, idUint, map[string]interface{}{
		"quota_tokens":  input.QuotaTokens,
		"rate_limit_rpm": input.RateLimitRPM,
		"status":        input.Status,
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   apiKey,
	})
}

func (h *AdminAPIHandler) DeleteAPIKey(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    ctx := c.Request.Context()
    if err := NewTenantService().DeleteAPIKey(ctx, idUint); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

	c.JSON(200, gin.H{
		"status": "ok",
		"message": "API key deleted",
	})
}

func (h *AdminAPIHandler) ListModels(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	modelsList, err := NewTenantService().ListModels(ctx, status)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   modelsList,
	})
}

func (h *AdminAPIHandler) CreateModel(c *gin.Context) {
	var input struct {
		Name             string `json:"name" binding:"required"`
		BackendType      string `json:"backend_type" binding:"required,oneof=ollama vllm llamacpp"`
		BackendURL       string `json:"backend_url" binding:"required"`
		BackendModelName string `json:"backend_model_name" binding:"required"`
		IsActive         bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	model, err := NewTenantService().CreateModel(ctx, input.Name, input.BackendType, input.BackendURL, input.BackendModelName, input.IsActive)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, gin.H{
		"status": "ok",
		"data":   model,
	})
}

func (h *AdminAPIHandler) UpdateModel(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    var input struct {
        Name             string `json:"name"`
        BackendType      string `json:"backend_type"`
        BackendURL       string `json:"backend_url"`
        BackendModelName string `json:"backend_model_name"`
        IsActive         bool   `json:"is_active"`
    }

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
    model, err := NewTenantService().UpdateModel(ctx, idUint, map[string]interface{}{
        "name":             input.Name,
        "backend_type":     input.BackendType,
        "backend_url":      input.BackendURL,
        "backend_model_name": input.BackendModelName,
        "is_active":        input.IsActive,
    })
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   model,
	})
}

func (h *AdminAPIHandler) DeleteModel(c *gin.Context) {
    id := c.Param("id")
    // convert string id to uint using fmt.Sscanf
    var idUint uint
    if _, err := fmt.Sscanf(id, "%d", &idUint); err != nil {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    ctx := c.Request.Context()
    if err := NewTenantService().DeleteModel(ctx, idUint); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"message": "Model deleted",
	})
}

func (h *AdminAPIHandler) GetUsageStats(c *gin.Context) {
	ctx := c.Request.Context()
	stats, err := NewTenantService().GetUsageStats(ctx, time.Time{}, time.Time{})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   stats,
	})
}

func (h *AdminAPIHandler) GetUsageStatsByTime(c *gin.Context) {
    ctx := c.Request.Context()
    startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	var start, end time.Time
	var err error

	if startTime != "" {
		start, err = time.Parse(time.RFC3339, startTime)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid start_time format"})
			return
		}
	}

	if endTime != "" {
		end, err = time.Parse(time.RFC3339, endTime)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid end_time format"})
			return
		}
	}

	stats, err := NewUsageService().GetStatsByTimeRange(ctx, start, end)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   stats,
	})
}

func (h *AdminAPIHandler) GetUsageStatsByKey(c *gin.Context) {
    ctx := c.Request.Context()
    startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	var start, end time.Time
	var err error

	if startTime != "" {
		start, err = time.Parse(time.RFC3339, startTime)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid start_time format"})
			return
		}
	}

	if endTime != "" {
		end, err = time.Parse(time.RFC3339, endTime)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid end_time format"})
			return
		}
	}

	stats, err := NewUsageService().GetStatsByAPIKey(ctx, start, end)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   stats,
	})
}

func (h *AdminAPIHandler) GetRecentLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		limit = parseInt(l)
	}

	logs, err := NewUsageService().GetRecentLogs(c.Request.Context(), limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status": "ok",
		"data":   logs,
	})
}

func parseInt(s string) int {
    if s == "" {
        return 100
    }
    var n int
    if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
        return 0
    }
    return n
}
