package admin

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/rs/zerolog/log"
    "modelgate/internal/database"
    "modelgate/internal/models"
    "gorm.io/gorm"
)

type BackendStatus string

const (
	StatusUnknown BackendStatus = "unknown"
	StatusOK      BackendStatus = "ok"
	StatusError   BackendStatus = "error"
)

type HealthCheckConfig struct {
	Timeout time.Duration
}

type HealthChecker struct {
	config HealthCheckConfig
}

func NewHealthChecker(config HealthCheckConfig) *HealthChecker {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	return &HealthChecker{config: config}
}

func (hc *HealthChecker) CheckBackend(ctx context.Context, model *models.Model) (*models.BackendHealth, error) {
	client := &http.Client{
		Timeout: hc.config.Timeout,
	}

	var checkURL string
	switch model.BackendType {
	case "ollama":
		checkURL = strings.TrimRight(model.BackendURL, "/") + "/v1/models"
	case "vllm":
		checkURL = strings.TrimRight(model.BackendURL, "/") + "/v1/models"
	case "llamacpp":
		checkURL = strings.TrimRight(model.BackendURL, "/") + "/v1/models"
	default:
		return &models.BackendHealth{
			BackendType:  model.BackendType,
			BackendURL:   model.BackendURL,
			IsValid:      false,
			LastError:    fmt.Sprintf("unsupported backend type: %s", model.BackendType),
			LastCheck:    time.Now(),
			ResponseTime: 0,
		}, nil
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return &models.BackendHealth{
			BackendType:  model.BackendType,
			BackendURL:   model.BackendURL,
			IsValid:      false,
			LastError:    err.Error(),
			LastCheck:    time.Now(),
			ResponseTime: 0,
		}, nil
	}

	resp, err := client.Do(req)
	responseTime := time.Since(start).Milliseconds()

	if err != nil {
		return &models.BackendHealth{
			BackendType:  model.BackendType,
			BackendURL:   model.BackendURL,
			IsValid:      false,
			LastError:    err.Error(),
			LastCheck:    time.Now(),
			ResponseTime: responseTime,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &models.BackendHealth{
			BackendType:  model.BackendType,
			BackendURL:   model.BackendURL,
			IsValid:      false,
			LastError:    fmt.Sprintf("backend returned status %d", resp.StatusCode),
			LastCheck:    time.Now(),
			ResponseTime: responseTime,
		}, nil
	}

	return &models.BackendHealth{
		BackendType:  model.BackendType,
		BackendURL:   model.BackendURL,
		IsValid:      true,
		LastError:    "",
		LastCheck:    time.Now(),
		ResponseTime: responseTime,
	}, nil
}

func SaveHealthCheckResult(ctx context.Context, health *models.BackendHealth) error {
    var existing models.BackendHealth
    err := database.DB.WithContext(ctx).Where("backend_type = ? AND backend_url = ?", health.BackendType, health.BackendURL).First(&existing).Error

    if err != nil {
        if err == gorm.ErrRecordNotFound {
            return database.DB.WithContext(ctx).Create(health).Error
        }
        return err
    }

    health.ID = existing.ID
    return database.DB.WithContext(ctx).Save(health).Error
}

func GetBackendHealth(ctx context.Context, backendType string) (*models.BackendHealth, error) {
	var health models.BackendHealth
	err := database.DB.Where("backend_type = ?", backendType).First(&health).Error
	if err != nil {
		return nil, err
	}
	return &health, nil
}

func GetAllBackendHealth(ctx context.Context) ([]models.BackendHealth, error) {
	var healths []models.BackendHealth
	err := database.DB.Find(&healths).Error
	return healths, err
}

func CheckAllBackends(ctx context.Context) error {
	var modelsList []models.Model
	err := database.DB.Where("is_active = ?", true).Find(&modelsList).Error
	if err != nil {
		return err
	}

	checker := NewHealthChecker(HealthCheckConfig{Timeout: 10 * time.Second})

	for _, model := range modelsList {
		health, err := checker.CheckBackend(ctx, &model)
		if err != nil {
			log.Error().Str("model", model.Name).Err(err).Msg("health check error")
			continue
		}

		if err := SaveHealthCheckResult(ctx, health); err != nil {
			log.Error().Str("model", model.Name).Err(err).Msg("failed to save health check result")
		} else {
			log.Info().Str("model", model.Name).Bool("is_valid", health.IsValid).Msg("health check completed")
		}
	}

	return nil
}

func StartHealthCheckScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				if err := CheckAllBackends(ctx); err != nil {
					log.Error().Err(err).Msg("health check scheduler error")
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
