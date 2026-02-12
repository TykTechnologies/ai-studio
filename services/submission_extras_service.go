package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// DuplicateCandidate represents a potential duplicate found during submission
type DuplicateCandidate struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	ResourceType     string `json:"resource_type"`
	MatchReason      string `json:"match_reason"`
	PrivacyScore     int    `json:"privacy_score"`
	CommunitySubmitted bool `json:"community_submitted"`
}

// CheckForDuplicates checks if a submission's payload matches existing resources
func (s *Service) CheckForDuplicates(resourceType string, payload models.JSONMap) ([]DuplicateCandidate, error) {
	var candidates []DuplicateCandidate

	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	switch resourceType {
	case models.SubmissionResourceTypeDatasource:
		candidates = append(candidates, s.findDuplicateDatasources(getString)...)
	case models.SubmissionResourceTypeTool:
		candidates = append(candidates, s.findDuplicateTools(getString)...)
	}

	return candidates, nil
}

func (s *Service) findDuplicateDatasources(getString func(string) string) []DuplicateCandidate {
	var candidates []DuplicateCandidate

	name := getString("name")
	connString := getString("db_conn_string")

	// Query DB directly instead of loading all records into memory
	if connString != "" {
		var connMatches []models.Datasource
		s.DB.Where("db_conn_string = ?", connString).Limit(10).Find(&connMatches)
		for _, ds := range connMatches {
			candidates = append(candidates, DuplicateCandidate{
				ID: ds.ID, Name: ds.Name,
				ResourceType:      models.SubmissionResourceTypeDatasource,
				MatchReason:       "Same database connection string",
				PrivacyScore:      ds.PrivacyScore,
				CommunitySubmitted: ds.CommunitySubmitted,
			})
		}
	}

	if name != "" {
		var nameMatches []models.Datasource
		s.DB.Where("LOWER(name) = LOWER(?)", name).Limit(10).Find(&nameMatches)
		for _, ds := range nameMatches {
			// Skip if already matched by connection string
			alreadyMatched := false
			for _, c := range candidates {
				if c.ID == ds.ID {
					alreadyMatched = true
					break
				}
			}
			if !alreadyMatched {
				candidates = append(candidates, DuplicateCandidate{
					ID: ds.ID, Name: ds.Name,
					ResourceType:      models.SubmissionResourceTypeDatasource,
					MatchReason:       "Same name (case-insensitive)",
					PrivacyScore:      ds.PrivacyScore,
					CommunitySubmitted: ds.CommunitySubmitted,
				})
			}
		}
	}

	return candidates
}

func (s *Service) findDuplicateTools(getString func(string) string) []DuplicateCandidate {
	var candidates []DuplicateCandidate

	name := getString("name")

	if name != "" {
		var nameMatches []models.Tool
		s.DB.Where("LOWER(name) = LOWER(?)", name).Limit(10).Find(&nameMatches)
		for _, tool := range nameMatches {
			candidates = append(candidates, DuplicateCandidate{
				ID: tool.ID, Name: tool.Name,
				ResourceType:      models.SubmissionResourceTypeTool,
				MatchReason:       "Same name (case-insensitive)",
				PrivacyScore:      tool.PrivacyScore,
				CommunitySubmitted: tool.CommunitySubmitted,
			})
		}
	}

	return candidates
}

// --- Orphan management ---

// HandleUserDeletionForUGC flags community resources owned by a deleted/deactivated user.
// Delegates to HandleUserDeletionForUGCTx with the default DB connection.
func (s *Service) HandleUserDeletionForUGC(userID uint) error {
	return s.HandleUserDeletionForUGCTx(s.DB, userID)
}

// HandleUserDeletionForUGCTx is the transaction-aware version of HandleUserDeletionForUGC.
func (s *Service) HandleUserDeletionForUGCTx(db *gorm.DB, userID uint) error {
	var orphanedCount int

	// Flag community datasources
	result := db.Model(&models.Datasource{}).
		Where("user_id = ? AND community_submitted = ?", userID, true).
		Update("active", false)
	if result.Error != nil {
		return fmt.Errorf("failed to deactivate community datasources: %w", result.Error)
	}
	orphanedCount += int(result.RowsAffected)

	// Count community tools (Tool model has no Active field, so we track via admin notification)
	var toolCount int64
	db.Model(&models.Tool{}).Where("user_id = ? AND community_submitted = ?", userID, true).Count(&toolCount)
	orphanedCount += int(toolCount)

	// Notify admins if there are orphaned community resources
	if orphanedCount > 0 && s.NotificationService != nil {
		// Get user info for the notification
		user := &models.User{}
		db.First(user, userID)

		title := fmt.Sprintf("Community resources orphaned: %d resources need reassignment", orphanedCount)
		notificationID := fmt.Sprintf("ugc_orphan_%d", userID)

		if err := s.NotificationService.Notify(
			notificationID, title, "",
			map[string]interface{}{
				"user_id":        userID,
				"user_name":      user.Name,
				"user_email":     user.Email,
				"orphaned_count": orphanedCount,
			},
			models.NotifyAdmins,
		); err != nil {
			logger.Warn(fmt.Sprintf("Failed to notify admins of orphaned UGC resources: %v", err))
		}
	}

	if orphanedCount > 0 {
		logger.Infof("User %d deletion: deactivated %d community resource(s)", userID, orphanedCount)
	}

	return nil
}

// GetOrphanedCommunityResources returns community resources whose owners have been deleted
func (s *Service) GetOrphanedCommunityResources() ([]models.Datasource, []models.Tool, error) {
	var orphanedDS []models.Datasource
	// Find community datasources where the owner user no longer exists
	if err := s.DB.
		Where("community_submitted = ? AND user_id NOT IN (SELECT id FROM users WHERE deleted_at IS NULL)", true).
		Find(&orphanedDS).Error; err != nil {
		return nil, nil, err
	}

	var orphanedTools []models.Tool
	if err := s.DB.
		Where("community_submitted = ? AND user_id NOT IN (SELECT id FROM users WHERE deleted_at IS NULL)", true).
		Find(&orphanedTools).Error; err != nil {
		return nil, nil, err
	}

	return orphanedDS, orphanedTools, nil
}

// --- Nominate from existing ---

// NominateExistingDatasource creates a draft submission pre-populated from an existing datasource
func (s *Service) NominateExistingDatasource(userID, datasourceID uint) (*models.Submission, error) {
	ds, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return nil, fmt.Errorf("datasource not found: %w", err)
	}

	// Verify the user has access (the datasource must be assigned to one of the user's apps)
	var count int64
	s.DB.Table("app_datasources").
		Joins("JOIN apps ON apps.id = app_datasources.app_id").
		Where("app_datasources.datasource_id = ? AND apps.user_id = ? AND apps.deleted_at IS NULL", datasourceID, userID).
		Count(&count)

	if count == 0 && ds.UserID != userID {
		return nil, fmt.Errorf("not authorized: datasource must be in one of your apps")
	}

	// Build payload from existing datasource (redact credentials)
	payload := models.JSONMap{
		"name":              ds.Name,
		"short_description": ds.ShortDescription,
		"long_description":  ds.LongDescription,
		"icon":              ds.Icon,
		"url":               ds.Url,
		"db_source_type":    ds.DBSourceType,
		"db_name":           ds.DBName,
		"embed_vendor":      string(ds.EmbedVendor),
		"embed_url":         ds.EmbedUrl,
		"embed_model":       ds.EmbedModel,
		// Credentials are intentionally omitted — submitter must re-enter them
	}

	// Extract tag names
	var tagNames []string
	for _, tag := range ds.Tags {
		tagNames = append(tagNames, tag.Name)
	}
	if len(tagNames) > 0 {
		payload["tags"] = tagNames
	}

	// Clamp privacy score to valid range for the submission
	privacyScore := ds.PrivacyScore
	if privacyScore < minPrivacyScore {
		privacyScore = minPrivacyScore
	} else if privacyScore > maxPrivacyScore {
		privacyScore = maxPrivacyScore
	}

	return s.CreateSubmission(
		userID,
		models.SubmissionResourceTypeDatasource,
		models.SubmissionStatusDraft,
		payload,
		nil,
		privacyScore,
		"",
		"", "", "", nil, "", "",
	)
}

// NominateExistingTool creates a draft submission pre-populated from an existing tool
func (s *Service) NominateExistingTool(userID, toolID uint) (*models.Submission, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, fmt.Errorf("tool not found: %w", err)
	}

	// Verify the user has access (the tool must be assigned to one of the user's apps)
	var count int64
	s.DB.Table("app_tools").
		Joins("JOIN apps ON apps.id = app_tools.app_id").
		Where("app_tools.tool_id = ? AND apps.user_id = ? AND apps.deleted_at IS NULL", toolID, userID).
		Count(&count)

	if count == 0 && tool.UserID != userID {
		return nil, fmt.Errorf("not authorized: tool must be in one of your apps")
	}

	// Build payload (redact auth credentials)
	payload := models.JSONMap{
		"name":                 tool.Name,
		"description":          tool.Description,
		"tool_type":            tool.ToolType,
		"oas_spec":             tool.OASSpec,
		"auth_schema_name":     tool.AuthSchemaName,
		"available_operations": tool.AvailableOperations,
		// AuthKey intentionally omitted — submitter must re-enter
	}

	// Clamp privacy score to valid range for the submission
	toolPrivacy := tool.PrivacyScore
	if toolPrivacy < minPrivacyScore {
		toolPrivacy = minPrivacyScore
	} else if toolPrivacy > maxPrivacyScore {
		toolPrivacy = maxPrivacyScore
	}

	return s.CreateSubmission(
		userID,
		models.SubmissionResourceTypeTool,
		models.SubmissionStatusDraft,
		payload,
		nil,
		toolPrivacy,
		"",
		"", "", "", nil, "", "",
	)
}
