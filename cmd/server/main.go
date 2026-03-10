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

	err = database.Init(cfg.Database.DSN)
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
	authMiddleware := middleware.NewAuthMiddleware(rateLimiter)

	gin.SetMode(cfg.Server.Mode)
	dataRouter := gin.New()
	dataRouter.Use(gin.Recovery())
	dataRouter.Use(middleware.Logger())
	dataRouter.Use(middleware.BodySizeLimit(cfg.MaxBodySize))

	dataRouter.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := dataRouter.Group("/v1")
	v1.Use(authMiddleware.Authenticate())
	v1.Use(authMiddleware.RateLimit())
	{
		v1.POST("/chat/completions", handleChatCompletions(gatewayService))
		v1.POST("/completions", handleCompletions(gatewayService))
		v1.GET("/models", handleListModels(gatewayService))
		v1.GET("/models/:id", handleRetrieveModel(gatewayService))
	}

	adminRouter := gin.New()
	adminRouter.Use(gin.Recovery())
	adminRouter.Use(middleware.Logger())
	adminRouter.Use(middleware.BodySizeLimit(cfg.MaxBodySize))

	adminRouter.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	adminGroup := adminRouter.Group("/admin")
	adminGroup.Use(authMiddleware.Authenticate())
	admin.RegisterRoutes(adminGroup)

	adminRouter.Static("/static/", "./admin")
	adminRouter.GET("/", func(c *gin.Context) {
		c.File("./admin/index.html")
	})

	dataSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: dataRouter,
	}

	adminSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Admin.Host, cfg.Admin.Port),
		Handler: adminRouter,
	}

	serverErrs := make(chan error, 2)

	go func() {
		logger.Info().Int("port", cfg.Server.Port).Msg("Data plane server starting")
		if err := dataSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrs <- fmt.Errorf("data plane server failed: %w", err)
		}
	}()

	go func() {
		logger.Info().Int("port", cfg.Admin.Port).Msg("Control plane server starting")
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrs <- fmt.Errorf("control plane server failed: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info().Msg("Shutdown signal received")
	case err := <-serverErrs:
		logger.Error().Err(err).Msg("Server runtime error received")
	}

	logger.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := dataSrv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Data plane server forced to shutdown")
	}

	if err := adminSrv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Control plane server forced to shutdown")
	}

	logger.Info().Msg("Server exited")
}

func handleChatCompletions(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawReq, err := decodeRawRequest(c)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			writeOpenAIError(c, http.StatusUnauthorized, "unauthorized", "authentication_error", nil)
			return
		}

		key := apiKeyModel.(*models.APIKey)
		messages, ok := rawReq["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			writeOpenAIError(c, http.StatusBadRequest, "messages must not be empty", "invalid_request_error", nil)
			return
		}

		modelName, err := resolveModelName(rawReq, key.DefaultModel)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		rawReq["model"] = modelName

		temperature, err := extractFloat64Field(rawReq, "temperature", 1.0)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if temperature < 0 || temperature > 2 {
			writeOpenAIError(c, http.StatusBadRequest, "temperature must be between 0 and 2", "invalid_request_error", nil)
			return
		}

		maxTokens, err := extractMaxTokens(rawReq)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if maxTokens < 0 {
			writeOpenAIError(c, http.StatusBadRequest, "max_tokens must be positive", "invalid_request_error", nil)
			return
		}

		topP, err := extractFloat64Field(rawReq, "top_p", 1.0)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if topP <= 0 || topP > 1 {
			writeOpenAIError(c, http.StatusBadRequest, "top_p must be between 0 and 1", "invalid_request_error", nil)
			return
		}

		n, err := extractIntField(rawReq, "n", 1)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if n <= 0 {
			writeOpenAIError(c, http.StatusBadRequest, "n must be a positive integer", "invalid_request_error", nil)
			return
		}

		stop, err := normalizeStopSequences(rawReq["stop"])
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

		stream, err := extractBoolField(rawReq, "stream", false)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Stream:      stream,
			Temperature: temperature,
			MaxTokens:   maxTokens,
			TopP:        topP,
			N:           n,
			Stop:        stop,
			RawBody:     rawReq,
		}

		streamStarted := false
		if stream {
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
			if stream && streamStarted {
				return
			}
			writeServiceError(c, err)
			return
		}

		if stream {
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
		rawReq, err := decodeRawRequest(c)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			writeOpenAIError(c, http.StatusUnauthorized, "unauthorized", "authentication_error", nil)
			return
		}

		key := apiKeyModel.(*models.APIKey)

		if _, ok := rawReq["prompt"]; !ok {
			writeOpenAIError(c, http.StatusBadRequest, "prompt is required", "invalid_request_error", nil)
			return
		}

		modelName, err := resolveModelName(rawReq, key.DefaultModel)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		rawReq["model"] = modelName

		temperature, err := extractFloat64Field(rawReq, "temperature", 1.0)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if temperature < 0 || temperature > 2 {
			writeOpenAIError(c, http.StatusBadRequest, "temperature must be between 0 and 2", "invalid_request_error", nil)
			return
		}

		maxTokens, err := extractMaxTokens(rawReq)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if maxTokens < 0 {
			writeOpenAIError(c, http.StatusBadRequest, "max_tokens must be positive", "invalid_request_error", nil)
			return
		}

		topP, err := extractFloat64Field(rawReq, "top_p", 1.0)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if topP <= 0 || topP > 1 {
			writeOpenAIError(c, http.StatusBadRequest, "top_p must be between 0 and 1", "invalid_request_error", nil)
			return
		}

		n, err := extractIntField(rawReq, "n", 1)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}
		if n <= 0 {
			writeOpenAIError(c, http.StatusBadRequest, "n must be a positive integer", "invalid_request_error", nil)
			return
		}

		stop, err := normalizeStopSequences(rawReq["stop"])
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

		stream, err := extractBoolField(rawReq, "stream", false)
		if err != nil {
			writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", nil)
			return
		}

		prompt, ok := rawReq["prompt"].(string)
		if !ok {
			prompt = ""
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Prompt:      prompt,
			Stream:      stream,
			Temperature: temperature,
			MaxTokens:   maxTokens,
			TopP:        topP,
			N:           n,
			Stop:        stop,
			RawBody:     rawReq,
		}

		streamStarted := false
		if stream {
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

		resp, err := svc.Completion(c.Request.Context(), adapterReq, key.ID, modelName)
		if err != nil {
			if stream && streamStarted {
				return
			}
			writeServiceError(c, err)
			return
		}

		if stream {
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

func handleRetrieveModel(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		modelName := c.Param("id")
		model, err := svc.GetModel(modelName)
		if err != nil {
			writeOpenAIError(c, http.StatusNotFound, err.Error(), "invalid_request_error", nil)
			return
		}

		c.JSON(http.StatusOK, buildOpenAIModelResponse(*model))
	}
}

func buildOpenAIModelsResponse(modelList []models.Model) adapters.OpenAIModelsResponse {
	resp := adapters.OpenAIModelsResponse{
		Object: "list",
		Data:   make([]adapters.Model, 0, len(modelList)),
	}

	for _, m := range modelList {
		resp.Data = append(resp.Data, buildOpenAIModelResponse(m))
	}

	return resp
}

func buildOpenAIModelResponse(m models.Model) adapters.Model {
	return adapters.Model{
		ID:      m.Name,
		Object:  "model",
		Created: m.CreatedAt.Unix(),
		OwnedBy: m.BackendType,
	}
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

func decodeRawRequest(c *gin.Context) (map[string]interface{}, error) {
	var payload map[string]interface{}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	return payload, nil
}

func resolveModelName(raw map[string]interface{}, defaultModel string) (string, error) {
	if rawModel, exists := raw["model"]; exists {
		modelName, ok := rawModel.(string)
		if !ok {
			return "", fmt.Errorf("model must be a string")
		}
		if strings.TrimSpace(modelName) != "" {
			return modelName, nil
		}
	}
	if strings.TrimSpace(defaultModel) == "" {
		return "", fmt.Errorf("model is required")
	}
	return defaultModel, nil
}

func extractBoolField(raw map[string]interface{}, key string, defaultValue bool) (bool, error) {
	value, exists := raw[key]
	if !exists {
		return defaultValue, nil
	}
	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return boolValue, nil
}

func extractFloat64Field(raw map[string]interface{}, key string, defaultValue float64) (float64, error) {
	value, exists := raw[key]
	if !exists || value == nil {
		return defaultValue, nil
	}
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}

func extractIntField(raw map[string]interface{}, key string, defaultValue int) (int, error) {
	value, exists := raw[key]
	if !exists || value == nil {
		return defaultValue, nil
	}

	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(parsed), nil
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(typed), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func extractMaxTokens(raw map[string]interface{}) (int, error) {
	if maxCompletionTokens, exists := raw["max_completion_tokens"]; exists && maxCompletionTokens != nil {
		return extractIntField(raw, "max_completion_tokens", 0)
	}
	return extractIntField(raw, "max_tokens", 0)
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
