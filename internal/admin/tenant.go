package admin

import (
	"context"
	"time"

	"modelgate/internal/database"
	"modelgate/internal/models"
)

type TenantService struct{}

func NewTenantService() *TenantService {
	return &TenantService{}
}

func (ts *TenantService) CreateUser(ctx context.Context, name, email string) (*models.User, error) {
    user := &models.User{
        Name:  name,
        Email: email,
    }
    if err := database.DB.Create(user).Error; err != nil {
        return nil, err
    }
    return user, nil
}

func (ts *TenantService) GetUser(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (ts *TenantService) ListUsers(ctx context.Context, status string) ([]models.User, error) {
	var users []models.User
	query := database.DB
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (ts *TenantService) UpdateUser(ctx context.Context, id uint, updates map[string]interface{}) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		return nil, err
	}

    if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
        return nil, err
    }
	return &user, nil
}

func (ts *TenantService) DeleteUser(ctx context.Context, id uint) error {
	return database.DB.Delete(&models.User{}, id).Error
}

func (ts *TenantService) CreateAPIKey(ctx context.Context, userID int, quotaTokens, rateLimitRPM int) (*models.APIKey, error) {
    apiKey := &models.APIKey{
        UserID:       userID,
        QuotaTokens:  quotaTokens,
        RateLimitRPM: rateLimitRPM,
    }
    if err := database.DB.Create(apiKey).Error; err != nil {
        return nil, err
    }
    return apiKey, nil
}

func (ts *TenantService) ListAPIKeys(ctx context.Context, status string) ([]models.APIKey, error) {
	var apiKeys []models.APIKey
	query := database.DB
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&apiKeys).Error; err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (ts *TenantService) UpdateAPIKey(ctx context.Context, id uint, updates map[string]interface{}) (*models.APIKey, error) {
	var apiKey models.APIKey
	if err := database.DB.First(&apiKey, id).Error; err != nil {
		return nil, err
	}

	if err := database.DB.Model(&apiKey).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (ts *TenantService) DeleteAPIKey(ctx context.Context, id uint) error {
	return database.DB.Delete(&models.APIKey{}, id).Error
}

func (ts *TenantService) CreateModel(ctx context.Context, name, backendType, backendURL, backendModelName string, isActive bool) (*models.Model, error) {
    model := &models.Model{
        Name:             name,
        BackendType:      backendType,
        BackendURL:       backendURL,
        BackendModelName: backendModelName,
        IsActive:         isActive,
    }
    if err := database.DB.Create(model).Error; err != nil {
        return nil, err
    }
    return model, nil
}

func (ts *TenantService) GetModel(ctx context.Context, id uint) (*models.Model, error) {
	var model models.Model
	if err := database.DB.First(&model, id).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

func (ts *TenantService) ListModels(ctx context.Context, status string) ([]models.Model, error) {
	var modelsList []models.Model
	query := database.DB
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&modelsList).Error; err != nil {
		return nil, err
	}
	return modelsList, nil
}

func (ts *TenantService) UpdateModel(ctx context.Context, id uint, updates map[string]interface{}) (*models.Model, error) {
    var model models.Model
    if err := database.DB.First(&model, id).Error; err != nil {
        return nil, err
    }

    if err := database.DB.Model(&model).Updates(updates).Error; err != nil {
        return nil, err
    }
	return &model, nil
}

func (ts *TenantService) DeleteModel(ctx context.Context, id uint) error {
	return database.DB.Delete(&models.Model{}, id).Error
}

func (ts *TenantService) GetUsageStats(ctx context.Context, startTime, endTime time.Time) (*UsageStats, error) {
	var stats UsageStats
	
	query := database.DB.Model(&models.APIKey{}).
		Select("SUM(used_tokens) as total_used_tokens, COUNT(*) as active_keys").
		Where("status = ?", "active")
		
	if err := query.Scan(&stats).Error; err != nil {
		return nil, err
	}

	return &stats, nil
}

type UsageStats struct {
	TotalUsedTokens int64 `json:"total_used_tokens"`
	ActiveKeys      int   `json:"active_keys"`
}
