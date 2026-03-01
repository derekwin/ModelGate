package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"modelgate/internal/auth"
	"modelgate/internal/config"
	"modelgate/internal/database"
	"modelgate/internal/models"
)

func RegisterRoutes(admin *gin.RouterGroup) {
	admin.GET("/verify", verifyAdmin)

	admin.Use(requireAdmin())
	admin.GET("/keys", listAPIKeys)
	admin.POST("/keys", createAPIKey)
	admin.PUT("/keys/:id", updateAPIKey)
	admin.DELETE("/keys/:id", deleteAPIKey)

	admin.GET("/models", listModels)
	admin.POST("/models", createModel)
	admin.PUT("/models/:id", updateModel)
	admin.DELETE("/models/:id", deleteModel)
	admin.POST("/models/sync", syncModels)
}

func verifyAdmin(c *gin.Context) {
	apiKeyModel, _ := c.Get("api_key_model")
	key := apiKeyModel.(*models.APIKey)
	c.JSON(http.StatusOK, gin.H{
		"admin": key.Admin,
		"name":  key.Name,
	})
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
		ID           uint   `json:"id"`
		Key          string `json:"key"`
		Name         string `json:"name"`
		Quota        int64  `json:"quota"`
		QuotaUsed    int64  `json:"quota_used"`
		RateLimit    int    `json:"rate_limit"`
		AllowedIPs   string `json:"allowed_ips"`
		Admin        bool   `json:"admin"`
		Tier         string `json:"tier"`
		DefaultModel string `json:"default_model"`
		Status       string `json:"status"`
	}

	resp := make([]keyResponse, len(keys))
	for i, k := range keys {
		resp[i] = keyResponse{
			ID:           k.ID,
			Key:          k.Key,
			Name:         k.Name,
			Quota:        k.Quota,
			QuotaUsed:    k.QuotaUsed,
			RateLimit:    k.RateLimit,
			AllowedIPs:   k.AllowedIPs,
			Admin:        k.Admin,
			Tier:         k.Tier,
			DefaultModel: k.DefaultModel,
			Status:       k.Status,
		}
	}
	c.JSON(http.StatusOK, resp)
}

func createAPIKey(c *gin.Context) {
	var input struct {
		Name         string `json:"name"`
		Quota        int64  `json:"quota"`
		RateLimit    int    `json:"rate_limit"`
		AllowedIPs   string `json:"allowed_ips"`
		Admin        bool   `json:"admin"`
		Tier         string `json:"tier"`
		DefaultModel string `json:"default_model"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tier := input.Tier
	if tier == "" {
		tier = "free"
	}

	apiKey := auth.GenerateAPIKey()
	keyHash := auth.HashAPIKey(apiKey)

	newKey := models.APIKey{
		Key:          apiKey,
		KeyHash:      keyHash,
		Name:         input.Name,
		Quota:        input.Quota,
		RateLimit:    input.RateLimit,
		AllowedIPs:   input.AllowedIPs,
		Admin:        input.Admin,
		Tier:         tier,
		DefaultModel: input.DefaultModel,
	}
	newKey.BaseModel.Status = "active"

	result := database.GetDB().Create(&newKey)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            newKey.ID,
		"key":           apiKey,
		"name":          newKey.Name,
		"quota":         newKey.Quota,
		"rate_limit":    newKey.RateLimit,
		"allowed_ips":   newKey.AllowedIPs,
		"admin":         newKey.Admin,
		"tier":          newKey.Tier,
		"default_model": newKey.DefaultModel,
	})
}

func updateAPIKey(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Name         string `json:"name"`
		Quota        int64  `json:"quota"`
		RateLimit    int    `json:"rate_limit"`
		AllowedIPs   string `json:"allowed_ips"`
		Status       string `json:"status"`
		Tier         string `json:"tier"`
		DefaultModel string `json:"default_model"`
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
	if input.Tier != "" {
		updates["tier"] = input.Tier
	}
	if input.DefaultModel != "" {
		updates["default_model"] = input.DefaultModel
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
		Enabled     *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(input.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if !isSupportedBackend(input.BackendType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported backend_type"})
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	newModel := models.Model{
		Name:        input.Name,
		BackendType: input.BackendType,
		BaseURL:     input.BaseURL,
		APIKey:      input.APIKey,
		Enabled:     enabled,
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
		Enabled     *bool  `json:"enabled"`
		Status      string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.BackendType != "" && !isSupportedBackend(input.BackendType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported backend_type"})
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
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}
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

func syncModels(c *gin.Context) {
	cfg := config.Get()
	adapters := cfg.Adapters

	type backend struct {
		name    string
		baseURL string
	}

	backends := []backend{
		{"ollama", adapters.Ollama.BaseURL},
		{"vllm", adapters.VLLM.BaseURL},
		{"llamacpp", adapters.LlamaCPP.BaseURL},
		{"openai", adapters.OpenAI.BaseURL},
		{"api3", adapters.API3.BaseURL},
	}

	type modelInfo struct {
		Name    string `json:"name"`
		Backend string `json:"backend_type"`
		BaseURL string `json:"base_url"`
		Enabled bool   `json:"enabled"`
	}

	var created []modelInfo

	for _, b := range backends {
		if b.baseURL == "" {
			continue
		}

		modelNames, err := fetchBackendModels(b.name, b.baseURL)
		if err != nil {
			continue
		}

		for _, name := range modelNames {
			var existing models.Model
			result := database.GetDB().Where("name = ? AND backend_type = ?", name, b.name).First(&existing)
			if result.Error == nil {
				continue
			}

			newModel := models.Model{
				Name:        name,
				BackendType: b.name,
				BaseURL:     b.baseURL,
				Enabled:     true,
			}
			newModel.Status = "active"

			if err := database.GetDB().Create(&newModel).Error; err != nil {
				continue
			}
			created = append(created, modelInfo{
				Name:    name,
				Backend: b.name,
				BaseURL: b.baseURL,
				Enabled: true,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Sync completed",
		"created": created,
	})
}

func fetchBackendModels(backend, baseURL string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url, err := backendModelsURL(backend, baseURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend %s returned status %d", backend, resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if backend == "ollama" {
		if models, ok := data["models"].([]interface{}); ok {
			result := make([]string, 0, len(models))
			for _, m := range models {
				if m, ok := m.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						result = append(result, name)
					}
				}
			}
			return result, nil
		}
	} else {
		if dataList, ok := data["data"].([]interface{}); ok {
			result := make([]string, 0, len(dataList))
			for _, m := range dataList {
				if m, ok := m.(map[string]interface{}); ok {
					if id, ok := m["id"].(string); ok {
						result = append(result, id)
					}
				}
			}
			return result, nil
		}
	}

	return nil, nil
}
