package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	MaxBodyMB      int    `mapstructure:"max_body_mb"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	SyncInterval   int    `mapstructure:"sync_interval"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type BackendConfig struct {
	Type      string `mapstructure:"type"`
	URL       string `mapstructure:"url"`
	ModelName string `mapstructure:"model_name"`
}

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Backends []BackendConfig `mapstructure:"backends"`
}

var (
	cfg       *Config
	once      sync.Once
	watcher   *fsnotify.Watcher
	watchOnce sync.Once
)

func LoadConfig(configPath string) error {
	var err error
	once.Do(func() {
		v := viper.New()
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")

		v.SetDefault("server.host", "0.0.0.0")
		v.SetDefault("server.port", 8080)
		v.SetDefault("server.max_body_mb", 5)
		v.SetDefault("server.timeout_seconds", 300)
		v.SetDefault("server.sync_interval", 60)
		v.SetDefault("database.path", "./data/modelgate.db")
		v.SetDefault("redis.addr", "localhost:6379")
		v.SetDefault("redis.password", "")
		v.SetDefault("redis.db", 0)
		v.SetDefault("backends", []BackendConfig{})

		v.SetEnvPrefix("MG")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()

		if err = v.ReadInConfig(); err != nil {
			log.Warn().Err(err).Msg("config: could not read config file, using defaults and env vars")
		}

		var c Config
		if err = v.Unmarshal(&c); err != nil {
			err = fmt.Errorf("config: unmarshal: %w", err)
			return
		}

		if c.Server.Port <= 0 {
			err = fmt.Errorf("config: invalid server.port")
			return
		}
		if c.Server.MaxBodyMB <= 0 {
			err = fmt.Errorf("config: invalid server.max_body_mb")
			return
		}
		if c.Server.TimeoutSeconds <= 0 {
			err = fmt.Errorf("config: invalid server.timeout_seconds")
			return
		}
		if c.Database.Path == "" {
			err = fmt.Errorf("config: invalid database.path")
			return
		}
		if c.Redis.Addr == "" {
			err = fmt.Errorf("config: invalid redis.addr")
			return
		}
		for i := range c.Backends {
			if c.Backends[i].ModelName == "" {
				c.Backends[i].ModelName = "default"
			}
		}
		cfg = &c
		log.Info().Str("host", c.Server.Host).Int("port", c.Server.Port).Msg("config loaded")
	})
	if cfg == nil {
		return err
	}
	return nil
}

func Get() *Config {
	return cfg
}

func WatchConfigChanges(callback func(*Config)) error {
	var err error
	watchOnce.Do(func() {
		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return
		}
	})
	if err != nil {
		return err
	}

	configPath := "configs/config.yaml"
	if err := watcher.Add(configPath); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Info().Str("file", event.Name).Msg("config file changed, reloading")
					if err := LoadConfig(configPath); err != nil {
						log.Error().Err(err).Msg("failed to reload config")
					} else {
						callback(cfg)
						log.Info().Msg("config reloaded successfully")
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error().Err(err).Msg("config watcher error")
			}
		}
	}()

	return nil
}

func (c *Config) GetServerHost() string { return c.Server.Host }
func (c *Config) GetServerPort() int    { return c.Server.Port }
func (c *Config) GetServerMaxBodyMB() int  { return c.Server.MaxBodyMB }
func (c *Config) GetMaxBodyMB() int { return c.Server.MaxBodyMB }
func (c *Config) GetServerTimeoutSeconds() int { return c.Server.TimeoutSeconds }
func (c *Config) GetSyncInterval() int { return c.Server.SyncInterval }
func (c *Config) GetDatabasePath() string { return c.Database.Path }
func (c *Config) GetRedisAddr() string { return c.Redis.Addr }
func (c *Config) GetRedisPassword() string { return c.Redis.Password }
func (c *Config) GetRedisDB() int { return c.Redis.DB }
func (c *Config) GetBackends() []BackendConfig { return c.Backends }
func GetTimeoutSeconds() int { return Get().Server.TimeoutSeconds }
func GetMaxBodyMB() int { return Get().Server.MaxBodyMB }
