package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"modelgate/internal/adapters"
	"modelgate/internal/config"
	"modelgate/internal/database"
	"modelgate/internal/middleware"
	"modelgate/internal/models"
	"modelgate/internal/service"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	// 解析命令行参数
	configFile := flag.String("c", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置文件
	if err := config.LoadConfig(*configFile); err != nil {
		log.Fatal().Err(err).Str("config", *configFile).Msg("failed to load config")
	}

	log.Info().Str("config", *configFile).Msg("config loaded")

	db, err := database.InitDatabase(config.Get().Database.Path)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init database")
	}
	defer database.CloseDatabase()

	if err := db.AutoMigrate(&models.User{}, &models.APIKey{}, &models.Model{}); err != nil {
		log.Fatal().Err(err).Msg("failed to migrate database")
	}

	rdb := initRedis()
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Warn().Err(err).Msg("Redis not available, rate limiting disabled")
	}
	defer rdb.Close()

	gateway := setupRouter(db, ctx)

	if err := config.WatchConfigChanges(func(c *config.Config) {
		log.Info().Msg("config changed, reloading gateway backends")
		if err := reloadGatewayBackends(gateway, ctx); err != nil {
			log.Error().Err(err).Msg("failed to reload gateway backends")
		}
	}); err != nil {
		log.Error().Err(err).Msg("failed to start config watcher")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctxShutdown, cancel := context.WithTimeout(context.Background(), time.Duration(config.Get().GetServerTimeoutSeconds())*time.Second)
	defer cancel()

	if err := gateway.Shutdown(ctxShutdown); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}

	log.Info().Str("host", config.Get().GetServerHost()).Int("port", config.Get().GetServerPort()).Msg("server exited gracefully")
}

func initRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     config.Get().Redis.Addr,
		Password: config.Get().Redis.Password,
		DB:       config.Get().Redis.DB,
	})
}

func setupRouter(db *gorm.DB, ctx context.Context) *GatewayWrapper {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(logToContext())
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.BodySizeLimiter(config.Get().GetMaxBodyMB()))

	backendConfig := make(map[string]service.BackendConfig)
	for _, bc := range config.Get().GetBackends() {
		backendConfig[bc.Type] = service.BackendConfig{
			Type:      bc.Type,
			URL:       bc.URL,
			ModelName: bc.ModelName,
		}
	}

	adapter := &adapters.OllamaAdapter{BaseURL: "http://localhost:11434"}

	gateway := service.NewGatewayService(map[string]service.Adapter{
		"ollama": adapter,
	}, backendConfig)

	if err := gateway.SyncModels(ctx); err != nil {
		log.Error().Err(err).Msg("failed to sync models on startup")
	}

	router.Use(func(c *gin.Context) {
		c.Set("gateway", gateway)
		c.Next()
	})

	router.Use(middleware.AuthMiddleware())
	router.Use(middleware.RateLimiterMiddleware())

	service.Route(router, gateway)

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "admin/index.html")
	})
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.Handle("/v1/", router)

	addr := fmt.Sprintf("%s:%d", config.Get().GetServerHost(), config.Get().GetServerPort())
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	return &GatewayWrapper{srv: srv, gateway: gateway}
}

func reloadGatewayBackends(gateway *GatewayWrapper, ctx context.Context) error {
	backendConfig := make(map[string]service.BackendConfig)
	for _, bc := range config.Get().GetBackends() {
		backendConfig[bc.Type] = service.BackendConfig{
			Type:      bc.Type,
			URL:       bc.URL,
			ModelName: bc.ModelName,
		}
	}

	gateway.gateway.SetBackendConfig(backendConfig)

	if err := gateway.gateway.SyncModels(ctx); err != nil {
		log.Error().Err(err).Msg("failed to sync models after config reload")
		return err
	}

	log.Info().Msg("gateway backends reloaded successfully")
	return nil
}

type GatewayWrapper struct {
	srv     *http.Server
	gateway *service.GatewayService
}

func (gw *GatewayWrapper) Shutdown(ctx context.Context) error {
	return gw.srv.Shutdown(ctx)
}

func logToContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctxLogger := log.Ctx(c.Request.Context()).With().
			Str("request_id", c.GetString("X-Request-ID")).
			Logger()
		c.Set("logger", ctxLogger)
		c.Request = c.Request.WithContext(ctxLogger.WithContext(c.Request.Context()))
		c.Next()
	}
}
