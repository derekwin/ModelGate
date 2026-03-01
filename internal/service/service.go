package service

import (
	"context"
	"fmt"

	"modelgate/internal/adapters"
	"modelgate/internal/database"
	"modelgate/internal/models"
	"modelgate/internal/usage"
)

type GatewayService struct {
	adapterFactory *adapters.AdapterFactory
}

func NewGatewayService(adapterFactory *adapters.AdapterFactory) *GatewayService {
	return &GatewayService{
		adapterFactory: adapterFactory,
	}
}

func (s *GatewayService) GetModel(name string) (*models.Model, error) {
	if name == "" {
		return nil, fmt.Errorf("model name is required")
	}

	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var model models.Model
	result := db.Where("name = ? AND enabled = ?", name, true).First(&model)
	if result.Error != nil {
		return nil, fmt.Errorf("model '%s' not found", name)
	}
	return &model, nil
}

func (s *GatewayService) ListModels() ([]models.Model, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var modelsList []models.Model
	result := db.Where("enabled = ?", true).Find(&modelsList)
	if result.Error != nil {
		return nil, result.Error
	}
	return modelsList, nil
}

func (s *GatewayService) ChatCompletion(ctx context.Context, req adapters.OpenAIRequest, apiKeyID uint, modelName string) (*adapters.OpenAIResponse, error) {
	model, err := s.GetModel(modelName)
	if err != nil {
		return nil, err
	}

	if model == nil {
		return nil, fmt.Errorf("model not found")
	}

	adapter := s.adapterFactory.GetAdapterForModel(*model)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported backend type: %s", model.BackendType)
	}
	resp, err := adapter.ChatCompletion(ctx, req, *model)
	if err != nil {
		return nil, err
	}

	if resp != nil && resp.Usage.TotalTokens > 0 {
		err = usage.RecordUsage(apiKeyID, modelName, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
		if err != nil {
			return resp, fmt.Errorf("failed to record usage: %w", err)
		}
		err = usage.UpdateAPIKeyQuota(apiKeyID, resp.Usage.TotalTokens)
		if err != nil {
			return resp, fmt.Errorf("failed to update quota: %w", err)
		}
	}

	return resp, nil
}

func (s *GatewayService) Completion(ctx context.Context, req adapters.OpenAIRequest, apiKeyID uint, modelName string) (*adapters.OpenAIResponse, error) {
	model, err := s.GetModel(modelName)
	if err != nil {
		return nil, err
	}

	if model == nil {
		return nil, fmt.Errorf("model not found")
	}

	adapter := s.adapterFactory.GetAdapterForModel(*model)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported backend type: %s", model.BackendType)
	}
	resp, err := adapter.Completion(ctx, req, *model)
	if err != nil {
		return nil, err
	}

	if resp != nil && resp.Usage.TotalTokens > 0 {
		err = usage.RecordUsage(apiKeyID, modelName, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
		if err != nil {
			return resp, fmt.Errorf("failed to record usage: %w", err)
		}
		err = usage.UpdateAPIKeyQuota(apiKeyID, resp.Usage.TotalTokens)
		if err != nil {
			return resp, fmt.Errorf("failed to update quota: %w", err)
		}
	}

	return resp, nil
}

func (s *GatewayService) ListBackendModels(ctx context.Context, modelName string) (*adapters.OpenAIModelsResponse, error) {
	model, err := s.GetModel(modelName)
	if err != nil {
		return nil, err
	}

	if model == nil {
		return nil, fmt.Errorf("model not found")
	}

	adapter := s.adapterFactory.GetAdapterForModel(*model)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported backend type: %s", model.BackendType)
	}
	return adapter.Models(ctx, *model)
}

func (s *GatewayService) CheckQuota(apiKeyID uint, requiredTokens int64) error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	var key models.APIKey
	result := db.First(&key, apiKeyID)
	if result.Error != nil {
		return fmt.Errorf("API key not found")
	}

	remaining := key.Quota - key.QuotaUsed
	if remaining < requiredTokens {
		return fmt.Errorf("insufficient quota: %d remaining, %d required", remaining, requiredTokens)
	}

	return nil
}
