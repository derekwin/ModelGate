package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"modelgate/internal/auth"
	"modelgate/internal/database"
	"modelgate/internal/models"
)

func RegisterRoutes(admin *gin.RouterGroup) {
	admin.Use(requireAdmin())
	admin.GET("/keys", listAPIKeys)
	admin.POST("/keys", createAPIKey)
	admin.PUT("/keys/:id", updateAPIKey)
	admin.DELETE("/keys/:id", deleteAPIKey)

	admin.GET("/models", listModels)
	admin.POST("/models", createModel)
	admin.PUT("/models/:id", updateModel)
	admin.DELETE("/models/:id", deleteModel)
}

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "admin access required", "type": "permission_denied"}})
			c.Abort()
			return
		}

		key := apiKeyModel.(*models.APIKey)
		if !key.Admin {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "admin access required", "type": "permission_denied"}})
			c.Abort()
			return
		}

		c.Next()
	}
}

func listAPIKeys(c *gin.Context) {
	var keys []models.APIKey
	result := database.GetDB().Find(&keys)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	type keyResponse struct {
		ID         uint   `json:"id"`
		Key        string `json:"key"`
		Name       string `json:"name"`
		Quota      int64  `json:"quota"`
		QuotaUsed  int64  `json:"quota_used"`
		RateLimit  int    `json:"rate_limit"`
		AllowedIPs string `json:"allowed_ips"`
		Admin      bool   `json:"admin"`
		Status     string `json:"status"`
	}

	resp := make([]keyResponse, len(keys))
	for i, k := range keys {
		resp[i] = keyResponse{
			ID:         k.ID,
			Key:        k.Key,
			Name:       k.Name,
			Quota:      k.Quota,
			QuotaUsed:  k.QuotaUsed,
			RateLimit:  k.RateLimit,
			AllowedIPs: k.AllowedIPs,
			Admin:      k.Admin,
			Status:     k.Status,
		}
	}
	c.JSON(http.StatusOK, resp)
}

func createAPIKey(c *gin.Context) {
	var input struct {
		Name       string `json:"name"`
		Quota      int64  `json:"quota"`
		RateLimit  int    `json:"rate_limit"`
		AllowedIPs string `json:"allowed_ips"`
		Admin      bool   `json:"admin"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiKey := auth.GenerateAPIKey()
	keyHash := auth.HashAPIKey(apiKey)

	newKey := models.APIKey{
		Key:        apiKey,
		KeyHash:    keyHash,
		Name:       input.Name,
		Quota:      input.Quota,
		RateLimit:  input.RateLimit,
		AllowedIPs: input.AllowedIPs,
		Admin:      input.Admin,
	}
	newKey.BaseModel.Status = "active"

	result := database.GetDB().Create(&newKey)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         newKey.ID,
		"key":        apiKey,
		"name":       newKey.Name,
		"quota":      newKey.Quota,
		"rate_limit": newKey.RateLimit,
		"admin":      newKey.Admin,
	})
}

func updateAPIKey(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Name       string `json:"name"`
		Quota      int64  `json:"quota"`
		RateLimit  int    `json:"rate_limit"`
		AllowedIPs string `json:"allowed_ips"`
		Status     string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var key models.APIKey
	result := database.GetDB().First(&key, id)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		return
	}

	updates := make(map[string]interface{})
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Quota > 0 {
		updates["quota"] = input.Quota
	}
	if input.RateLimit > 0 {
		updates["rate_limit"] = input.RateLimit
	}
	if input.AllowedIPs != "" {
		updates["allowed_ips"] = input.AllowedIPs
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}

	result = database.GetDB().Model(&key).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, key)
}

func deleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	result := database.GetDB().Delete(&models.APIKey{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API key deleted"})
}

func listModels(c *gin.Context) {
	var models []models.Model
	result := database.GetDB().Find(&models)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, models)
}

func createModel(c *gin.Context) {
	var input struct {
		Name        string `json:"name"`
		BackendType string `json:"backend_type"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		Enabled     bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newModel := models.Model{
		Name:        input.Name,
		BackendType: input.BackendType,
		BaseURL:     input.BaseURL,
		APIKey:      input.APIKey,
		Enabled:     input.Enabled,
	}
	newModel.BaseModel.Status = "active"

	result := database.GetDB().Create(&newModel)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusCreated, newModel)
}

func updateModel(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Name        string `json:"name"`
		BackendType string `json:"backend_type"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		Enabled     bool   `json:"enabled"`
		Status      string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var model models.Model
	result := database.GetDB().First(&model, id)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Model not found"})
		return
	}

	updates := make(map[string]interface{})
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.BackendType != "" {
		updates["backend_type"] = input.BackendType
	}
	if input.BaseURL != "" {
		updates["base_url"] = input.BaseURL
	}
	if input.APIKey != "" {
		updates["api_key"] = input.APIKey
	}
	updates["enabled"] = input.Enabled
	if input.Status != "" {
		updates["status"] = input.Status
	}

	result = database.GetDB().Model(&model).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, model)
}

func deleteModel(c *gin.Context) {
	id := c.Param("id")
	result := database.GetDB().Delete(&models.Model{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Model deleted"})
}
