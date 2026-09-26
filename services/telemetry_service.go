package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

type TelemetryService struct {
	DB *gorm.DB
	// tokens keeps the token sums incrementally, so each collection reads
	// only the chat records added since the last one.
	tokens *models.TokenTotals
}

func NewTelemetryService(db *gorm.DB) *TelemetryService {
	return &TelemetryService{
		DB:     db,
		tokens: models.NewTokenTotals(),
	}
}

// tokenTotals returns the tokens of every chat record, overall and by
// interaction type.
func (s *TelemetryService) tokenTotals() (int64, map[models.InteractionType]int64, error) {
	return s.tokens.Read(s.DB)
}

func (s *TelemetryService) GetLLMStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}
	llms := &models.LLMs{}

	llmCount, err := llms.GetLLMCount(s.DB)
	if err != nil {
		return nil, err
	}

	stats["llms_count"] = llmCount

	totalTokens, _, err := s.tokenTotals()
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

	_, byType, err := s.tokenTotals()
	if err != nil {
		return nil, err
	}

	stats["total_tokens"] = byType[models.ProxyInteraction]

	return stats, nil
}

func (s *TelemetryService) GetUserStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	userCounts, err := models.GetUserCounts(s.DB)
	if err != nil {
		return nil, err
	}

	stats["users_count"] = userCounts.UserCount
	stats["admin_users"] = userCounts.AdminCount
	stats["developers"] = userCounts.DeveloperCount
	stats["chat_users"] = userCounts.ChatUserCount

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

	_, byType, err := s.tokenTotals()
	if err != nil {
		return nil, err
	}

	stats["total_tokens"] = byType[models.ChatInteraction]

	return stats, nil
}
