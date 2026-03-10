package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig     `mapstructure:"server"`
	Database    DatabaseConfig   `mapstructure:"database"`
	Redis       RedisConfig      `mapstructure:"redis"`
	RateLimit   RateLimitConfig  `mapstructure:"rate_limit"`
	Quota       QuotaConfig      `mapstructure:"quota"`
	Timeout     time.Duration    `mapstructure:"timeout"`
	MaxBodySize int64            `mapstructure:"max_body_size"`
	Adapters    AdaptersConfig   `mapstructure:"adapters"`
	Resilience  ResilienceConfig `mapstructure:"resilience"`
	Log         LogConfig        `mapstructure:"log"`
	Admin       AdminConfig      `mapstructure:"admin"`
}

type AdminConfig struct {
	APIKey string `mapstructure:"api_key"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	DSN string `mapstructure:"dsn"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RateLimitConfig struct {
	RPM   int `mapstructure:"rpm"`
	Burst int `mapstructure:"burst"`
}

type QuotaConfig struct {
	DefaultTokens int64 `mapstructure:"default_tokens"`
}

type AdaptersConfig struct {
	Ollama   AdapterConfig `mapstructure:"ollama"`
	VLLM     AdapterConfig `mapstructure:"vllm"`
	LlamaCPP AdapterConfig `mapstructure:"llamacpp"`
	OpenAI   AdapterConfig `mapstructure:"openai"`
	API3     AdapterConfig `mapstructure:"api3"`
}

type AdapterConfig struct {
	BaseURL      string   `mapstructure:"base_url"`
	FallbackURLs []string `mapstructure:"fallback_urls"`
}

type ResilienceConfig struct {
	RetryAttempts  int                  `mapstructure:"retry_attempts"`
	RetryBackoff   time.Duration        `mapstructure:"retry_backoff"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

type CircuitBreakerConfig struct {
	FailureThreshold    int           `mapstructure:"failure_threshold"`
	OpenTimeout         time.Duration `mapstructure:"open_timeout"`
	HalfOpenMaxRequests int           `mapstructure:"half_open_max_requests"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

var (
	cfg  *Config
	once sync.Once
)

func Load(path string) (*Config, error) {
	var err error
	once.Do(func() {
		viper.SetConfigType("yaml")
		viper.SetConfigFile(path)
		viper.SetEnvPrefix("MG")
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		viper.AutomaticEnv()

		viper.SetDefault("server.host", "0.0.0.0")
		viper.SetDefault("server.port", 8080)
		viper.SetDefault("server.mode", "release")
		viper.SetDefault("database.dsn", "host=localhost user=modelgate password=modelgate dbname=modelgate port=5432 sslmode=disable TimeZone=UTC")
		viper.SetDefault("redis.addr", "localhost:6379")
		viper.SetDefault("redis.password", "")
		viper.SetDefault("redis.db", 0)
		viper.SetDefault("rate_limit.rpm", 60)
		viper.SetDefault("rate_limit.burst", 10)
		viper.SetDefault("quota.default_tokens", 1000000)
		viper.SetDefault("timeout", "300s")
		viper.SetDefault("max_body_size", 5242880)
		viper.SetDefault("resilience.retry_attempts", 2)
		viper.SetDefault("resilience.retry_backoff", "200ms")
		viper.SetDefault("resilience.circuit_breaker.failure_threshold", 5)
		viper.SetDefault("resilience.circuit_breaker.open_timeout", "30s")
		viper.SetDefault("resilience.circuit_breaker.half_open_max_requests", 1)

		e := viper.ReadInConfig()
		if e != nil {
			err = fmt.Errorf("failed to read config: %w", e)
			return
		}

		cfg = &Config{}
		e = viper.Unmarshal(cfg)
		if e != nil {
			err = fmt.Errorf("failed to unmarshal config: %w", e)
			return
		}
	})
	return cfg, err
}

func Get() *Config {
	return cfg
}

func Reload() error {
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	reloaded := &Config{}
	if err := viper.Unmarshal(reloaded); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg = reloaded
	return nil
}
