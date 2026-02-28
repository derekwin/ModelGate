package usage

import (
	"modelgate/internal/database"
	"modelgate/internal/models"
)

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
	result := database.GetDB().Model(&models.APIKey{}).
		Where("id = ?", apiKeyID).
		UpdateColumn("quota_used", database.GetDB().Raw("quota_used + ?", tokens))

	return result.Error
}
