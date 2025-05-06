package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

type TelemetryService struct {
	DB *gorm.DB
}

func NewTelemetryService(db *gorm.DB) *TelemetryService {
	return &TelemetryService{
		DB: db,
	}
}

func (s *TelemetryService) GetLLMStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	llms := &models.LLMs{}
	llmCount, err := llms.GetLLMCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["llms_count"] = llmCount

	totalTokens, err := models.GetTotalTokens(s.DB)
	if err != nil {
		return nil, err
	}
	stats["total_tokens"] = totalTokens

	return stats, nil
}

func (s *TelemetryService) GetAppStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	apps := &models.Apps{}
	appCount, err := apps.GetAppCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["apps_count"] = appCount

	proxyTokens, err := models.GetTotalTokensByInteractionType(s.DB, models.ProxyInteraction)
	if err != nil {
		return nil, err
	}
	stats["total_tokens"] = proxyTokens

	return stats, nil
}

func (s *TelemetryService) GetUserStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	userCount, err := models.GetUserCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["users_count"] = userCount

	adminCount, err := models.GetAdminCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["admin_users"] = adminCount

	devCount, err := models.GetDeveloperCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["developers"] = devCount

	chatUserCount, err := models.GetChatUserCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["chat_users"] = chatUserCount

	groupCount, err := models.GetUserGroupCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["user_groups"] = groupCount

	return stats, nil
}

func (s *TelemetryService) GetChatStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	chats := &models.Chats{}
	chatCount, err := chats.GetChatCount(s.DB)
	if err != nil {
		return nil, err
	}
	stats["chats_count"] = chatCount

	chatTokens, err := models.GetTotalTokensByInteractionType(s.DB, models.ChatInteraction)
	if err != nil {
		return nil, err
	}
	stats["total_tokens"] = chatTokens

	return stats, nil
}
