package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

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

	// Deactivate community tools
	toolResult := db.Model(&models.Tool{}).
		Where("user_id = ? AND community_submitted = ?", userID, true).
		Update("active", false)
	if toolResult.Error != nil {
		return fmt.Errorf("failed to deactivate community tools: %w", toolResult.Error)
	}
	orphanedCount += int(toolResult.RowsAffected)

	// Notify admins if there are orphaned community resources
	if orphanedCount > 0 && s.NotificationService != nil {
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
