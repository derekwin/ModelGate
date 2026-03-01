package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"modelgate/internal/adapters"
	"modelgate/internal/admin"
	"modelgate/internal/config"
	"modelgate/internal/database"
	"modelgate/internal/limiter"
	"modelgate/internal/middleware"
	"modelgate/internal/models"
	"modelgate/internal/service"
)

var logger zerolog.Logger

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	if cfg.Log.Format == "json" {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	}

	level, _ := zerolog.ParseLevel(cfg.Log.Level)
	logger.Level(level)

	logger.Info().Msg("Starting ModelGate...")

	err = database.Init(cfg.Database.Path)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer database.Close()

	if err := database.EnsureAdminKey(cfg.Admin.APIKey); err != nil {
		logger.Warn().Err(err).Msg("Failed to ensure admin key")
	}

	rateLimiter, err := limiter.NewRateLimiter(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.RateLimit.RPM,
		cfg.RateLimit.Burst,
	)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to initialize rate limiter, running without rate limiting")
	}
	if rateLimiter != nil {
		defer rateLimiter.Close()
	}

	adapterFactory := adapters.NewAdapterFactory(cfg)
	gatewayService := service.NewGatewayService(adapterFactory)

	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.Use(middleware.Logger())
	r.Use(middleware.BodySizeLimit(cfg.MaxBodySize))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authMiddleware := middleware.NewAuthMiddleware(rateLimiter)

	v1 := r.Group("/v1")
	v1.Use(authMiddleware.Authenticate())
	v1.Use(authMiddleware.RateLimit())
	{
		v1.POST("/chat/completions", handleChatCompletions(gatewayService))
		v1.POST("/completions", handleCompletions(gatewayService))
		v1.GET("/models", handleListModels(gatewayService))
	}

	adminGroup := r.Group("/admin")
	adminGroup.Use(authMiddleware.Authenticate())
	admin.RegisterRoutes(adminGroup)

	r.Static("/static/", "./admin")
	r.GET("/", func(c *gin.Context) {
		c.File("./admin/index.html")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: r,
	}

	go func() {
		logger.Info().Int("port", cfg.Server.Port).Msg("Server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Server exited")
}

func handleChatCompletions(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model       string                   `json:"model"`
			Messages    []map[string]interface{} `json:"messages" binding:"required"`
			Stream      bool                     `json:"stream"`
			Temperature *float64                 `json:"temperature,omitempty"`
			MaxTokens   *int                     `json:"max_tokens,omitempty"`
			TopP        *float64                 `json:"top_p,omitempty"`
			N           *int                     `json:"n,omitempty"`
			Stop        interface{}              `json:"stop,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			writeOpenAIError(c, http.StatusUnauthorized, "unauthorized", "authentication_error", nil)
			return
		}

		key := apiKeyModel.(*models.APIKey)
		if len(req.Messages) == 0 {
			writeOpenAIError(c, http.StatusBadRequest, "messages must not be empty", "invalid_request_error", nil)
			return
		}

		// Auto-select model based on tier if not provided
		modelName := req.Model
		if modelName == "" {
			if key.DefaultModel != "" {
				modelName = key.DefaultModel
			} else {
				writeOpenAIError(c, http.StatusBadRequest, "model is required", "invalid_request_error", nil)
				return
			}
		}

		temperature := 1.0
		if req.Temperature != nil {
			temperature = *req.Temperature
		}
		if temperature < 0 || temperature > 2 {
			writeOpenAIError(c, http.StatusBadRequest, "temperature must be between 0 and 2", "invalid_request_error", nil)
			return
		}

		maxTokens := 0
		if req.MaxTokens != nil {
			maxTokens = *req.MaxTokens
		}
		if maxTokens < 0 {
			writeOpenAIError(c, http.StatusBadRequest, "max_tokens must be positive", "invalid_request_error", nil)
			return
		}

		topP := 1.0
		if req.TopP != nil {
			topP = *req.TopP
		}
		if topP <= 0 || topP > 1 {
			writeOpenAIError(c, http.StatusBadRequest, "top_p must be between 0 and 1", "invalid_request_error", nil)
			return
		}

		n := 1
		if req.N != nil {
			n = *req.N
		}
		if n <= 0 {
			writeOpenAIError(c, http.StatusBadRequest, "n must be a positive integer", "invalid_request_error", nil)
			return
		}

		stop, err := normalizeStopSequences(req.Stop)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		if maxTokens > 0 {
			if err := svc.CheckQuota(key.ID, int64(maxTokens)); err != nil {
				writeOpenAIError(c, http.StatusForbidden, err.Error(), "insufficient_quota", nil)
				return
			}
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Stream:      req.Stream,
			Temperature: temperature,
			MaxTokens:   maxTokens,
			TopP:        topP,
			N:           n,
			Stop:        stop,
		}

		for _, msg := range req.Messages {
			role, ok := msg["role"].(string)
			if !ok {
				writeOpenAIError(c, http.StatusBadRequest, "message role must be a string", "invalid_request_error", nil)
				return
			}

			var content string
			switch v := msg["content"].(type) {
			case string:
				content = v
			case []interface{}:
				// For multimodal content, extract text from first text block
				for _, item := range v {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if text, ok := itemMap["text"].(string); ok {
							content = text
							break
						}
					}
				}
			default:
				content = fmt.Sprintf("%v", v)
			}

			adapterReq.Messages = append(adapterReq.Messages, adapters.ChatMessage{
				Role:    role,
				Content: content,
			})
		}

		streamStarted := false
		if req.Stream {
			adapterReq.StreamFunc = func(chunk string) {
				if !streamStarted {
					streamStarted = true
					c.Header("Content-Type", "text/event-stream")
					c.Header("Cache-Control", "no-cache")
					c.Header("Connection", "keep-alive")
					c.Status(http.StatusOK)
				}
				_, _ = c.Writer.WriteString(chunk)
				c.Writer.Flush()
			}
		}

		resp, err := svc.ChatCompletion(c.Request.Context(), adapterReq, key.ID, modelName)
		if err != nil {
			if req.Stream && streamStarted {
				errData, _ := json.Marshal(gin.H{
					"error": gin.H{
						"message": err.Error(),
						"type":    "server_error",
					},
				})
				_, _ = c.Writer.WriteString("data: " + string(errData) + "\n\n")
				_, _ = c.Writer.WriteString("data: [DONE]\n\n")
				c.Writer.Flush()
				return
			}
			writeServiceError(c, err)
			return
		}

		if req.Stream {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.Status(http.StatusOK)
			_, _ = c.Writer.WriteString("data: [DONE]\n\n")
			c.Writer.Flush()
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func handleCompletions(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model       string      `json:"model"`
			Prompt      string      `json:"prompt"`
			Stream      bool        `json:"stream"`
			Temperature *float64    `json:"temperature,omitempty"`
			MaxTokens   *int        `json:"max_tokens,omitempty"`
			TopP        *float64    `json:"top_p,omitempty"`
			N           *int        `json:"n,omitempty"`
			Stop        interface{} `json:"stop,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			writeOpenAIError(c, http.StatusUnauthorized, "unauthorized", "authentication_error", nil)
			return
		}

		key := apiKeyModel.(*models.APIKey)

		// Auto-select model based on tier if not provided
		modelName := req.Model
		if modelName == "" {
			if key.DefaultModel != "" {
				modelName = key.DefaultModel
			} else {
				writeOpenAIError(c, http.StatusBadRequest, "model is required", "invalid_request_error", nil)
				return
			}
		}

		temperature := 1.0
		if req.Temperature != nil {
			temperature = *req.Temperature
		}
		if temperature < 0 || temperature > 2 {
			writeOpenAIError(c, http.StatusBadRequest, "temperature must be between 0 and 2", "invalid_request_error", nil)
			return
		}

		maxTokens := 0
		if req.MaxTokens != nil {
			maxTokens = *req.MaxTokens
		}
		if maxTokens < 0 {
			writeOpenAIError(c, http.StatusBadRequest, "max_tokens must be positive", "invalid_request_error", nil)
			return
		}

		topP := 1.0
		if req.TopP != nil {
			topP = *req.TopP
		}
		if topP <= 0 || topP > 1 {
			writeOpenAIError(c, http.StatusBadRequest, "top_p must be between 0 and 1", "invalid_request_error", nil)
			return
		}

		n := 1
		if req.N != nil {
			n = *req.N
		}
		if n <= 0 {
			writeOpenAIError(c, http.StatusBadRequest, "n must be a positive integer", "invalid_request_error", nil)
			return
		}

		stop, err := normalizeStopSequences(req.Stop)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		if maxTokens > 0 {
			if err := svc.CheckQuota(key.ID, int64(maxTokens)); err != nil {
				writeOpenAIError(c, http.StatusForbidden, err.Error(), "insufficient_quota", nil)
				return
			}
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Prompt:      req.Prompt,
			Stream:      req.Stream,
			Temperature: temperature,
			MaxTokens:   maxTokens,
			TopP:        topP,
			N:           n,
			Stop:        stop,
		}

		resp, err := svc.Completion(c.Request.Context(), adapterReq, key.ID, modelName)
		if err != nil {
			writeServiceError(c, err)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func handleListModels(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		modelName := c.Query("model")
		if modelName == "" {
			models, err := svc.ListModels()
			if err != nil {
				writeOpenAIError(c, http.StatusInternalServerError, err.Error(), "server_error", nil)
				return
			}
			c.JSON(http.StatusOK, buildOpenAIModelsResponse(models))
			return
		}

		resp, err := svc.ListBackendModels(c.Request.Context(), modelName)
		if err != nil {
			writeServiceError(c, err)
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func buildOpenAIModelsResponse(modelList []models.Model) adapters.OpenAIModelsResponse {
	resp := adapters.OpenAIModelsResponse{
		Object: "list",
		Data:   make([]adapters.Model, 0, len(modelList)),
	}

	for _, m := range modelList {
		resp.Data = append(resp.Data, adapters.Model{
			ID:      m.Name,
			Object:  "model",
			Created: m.CreatedAt.Unix(),
			OwnedBy: m.BackendType,
		})
	}

	return resp
}

func normalizeStopSequences(raw interface{}) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []string{v}, nil
	case []interface{}:
		stops := make([]string, 0, len(v))
		for _, item := range v {
			stop, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("stop array must contain only strings")
			}
			if strings.TrimSpace(stop) != "" {
				stops = append(stops, stop)
			}
		}
		return stops, nil
	case []string:
		stops := make([]string, 0, len(v))
		for _, stop := range v {
			if strings.TrimSpace(stop) != "" {
				stops = append(stops, stop)
			}
		}
		return stops, nil
	default:
		return nil, fmt.Errorf("stop must be a string or array of strings")
	}
}

func writeServiceError(c *gin.Context, err error) {
	var apiErr *adapters.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.HTTPStatus
		if status == 0 {
			status = http.StatusBadGateway
		}
		errType := apiErr.Type
		if errType == "" {
			if status >= 500 {
				errType = "server_error"
			} else {
				errType = "invalid_request_error"
			}
		}
		writeOpenAIError(c, status, apiErr.Message, errType, apiErr.Code)
		return
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "not found"):
		writeOpenAIError(c, http.StatusBadRequest, message, "invalid_request_error", nil)
	case strings.Contains(message, "unsupported backend"):
		writeOpenAIError(c, http.StatusBadRequest, message, "invalid_request_error", nil)
	case strings.Contains(message, "quota"):
		writeOpenAIError(c, http.StatusForbidden, message, "insufficient_quota", nil)
	default:
		writeOpenAIError(c, http.StatusInternalServerError, message, "server_error", nil)
	}
}

func writeOpenAIError(c *gin.Context, status int, message, errorType string, code interface{}) {
	errBody := gin.H{
		"message": message,
		"type":    errorType,
	}
	if code != nil {
		errBody["code"] = code
	}

	c.JSON(status, gin.H{"error": errBody})
}
