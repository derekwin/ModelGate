package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	defer rateLimiter.Close()

	adapterFactory := adapters.NewAdapterFactory(cfg)
	gatewayService := service.NewGatewayService(adapterFactory)

	gin.SetMode(cfg.Server.Mode)
	r := gin.Default()

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
			Model       string              `json:"model"`
			Messages    []map[string]string `json:"messages" binding:"required"`
			Stream      bool                `json:"stream"`
			Temperature float64             `json:"temperature"`
			MaxTokens   int                 `json:"max_tokens"`
			TopP        float64             `json:"top_p"`
			N           int                 `json:"n"`
			Stop        []string            `json:"stop"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "unauthorized", "type": "authentication_error"}})
			return
		}

		key := apiKeyModel.(*models.APIKey)

		// Auto-select model based on tier if not provided
		modelName := req.Model
		if modelName == "" {
			if key.DefaultModel != "" {
				modelName = key.DefaultModel
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "model is required", "type": "invalid_request_error"}})
				return
			}
		}

		if req.Temperature < 0 || req.Temperature > 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "temperature must be between 0 and 2", "type": "invalid_request_error"}})
			return
		}

		if req.MaxTokens < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "max_tokens must be positive", "type": "invalid_request_error"}})
			return
		}

		if req.MaxTokens > 0 {
			if err := svc.CheckQuota(key.ID, int64(req.MaxTokens)); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": err.Error(), "type": "quota_exceeded"}})
				return
			}
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
			N:           req.N,
			Stop:        req.Stop,
		}

		for _, msg := range req.Messages {
			adapterReq.Messages = append(adapterReq.Messages, adapters.ChatMessage{
				Role:    msg["role"],
				Content: msg["content"],
			})
		}

		resp, err := svc.ChatCompletion(c.Request.Context(), adapterReq, key.ID, modelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "server_error"}})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

func handleCompletions(svc *service.GatewayService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model       string   `json:"model"`
			Prompt      string   `json:"prompt"`
			Stream      bool     `json:"stream"`
			Temperature float64  `json:"temperature"`
			MaxTokens   int      `json:"max_tokens"`
			TopP        float64  `json:"top_p"`
			N           int      `json:"n"`
			Stop        []string `json:"stop"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
			return
		}

		apiKeyModel, exists := c.Get("api_key_model")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "unauthorized", "type": "authentication_error"}})
			return
		}

		key := apiKeyModel.(*models.APIKey)

		// Auto-select model based on tier if not provided
		modelName := req.Model
		if modelName == "" {
			if key.DefaultModel != "" {
				modelName = key.DefaultModel
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "model is required", "type": "invalid_request_error"}})
				return
			}
		}

		if req.Temperature < 0 || req.Temperature > 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "temperature must be between 0 and 2", "type": "invalid_request_error"}})
			return
		}

		if req.MaxTokens < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "max_tokens must be positive", "type": "invalid_request_error"}})
			return
		}

		if req.MaxTokens > 0 {
			if err := svc.CheckQuota(key.ID, int64(req.MaxTokens)); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": err.Error(), "type": "quota_exceeded"}})
				return
			}
		}

		adapterReq := adapters.OpenAIRequest{
			Model:       modelName,
			Prompt:      req.Prompt,
			Stream:      req.Stream,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
			N:           req.N,
			Stop:        req.Stop,
		}

		resp, err := svc.Completion(c.Request.Context(), adapterReq, key.ID, modelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "server_error"}})
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
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": models})
			return
		}

		resp, err := svc.ListBackendModels(c.Request.Context(), modelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}
