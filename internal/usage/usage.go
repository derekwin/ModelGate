package usage

import (
	"errors"

	"modelgate/internal/database"
	"modelgate/internal/models"
)

var ErrInsufficientQuota = errors.New("insufficient quota")

func RecordUsage(apiKeyID uint, model string, promptTokens, completionTokens, totalTokens int64) error {
	record := models.UsageRecord{
		APIKeyID:         apiKeyID,
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}

	result := database.GetDB().Create(&record)
	return result.Error
}

func GetAPIKeyUsage(apiKeyID uint) (int64, error) {
	var total int64
	result := database.GetDB().Model(&models.UsageRecord{}).
		Where("api_key_id = ?", apiKeyID).
		Select("COALESCE(SUM(total_tokens), 0)").
		Scan(&total)

	if result.Error != nil {
		return 0, result.Error
	}
	return total, nil
}

func UpdateAPIKeyQuota(apiKeyID uint, tokens int64) error {
	if tokens <= 0 {
		return nil
	}

	result := database.GetDB().Model(&models.APIKey{}).
		Where("id = ? AND quota_used + ? <= quota", apiKeyID, tokens).
		UpdateColumn("quota_used", database.GetDB().Raw("quota_used + ?", tokens))

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInsufficientQuota
	}

	return result.Error
}
