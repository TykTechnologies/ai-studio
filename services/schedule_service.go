package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

var (
	ErrScheduleNotFound        = errors.New("schedule not found")
	ErrScheduleAlreadyExists   = errors.New("schedule with this ID already exists")
	ErrInvalidCronExpression   = errors.New("invalid cron expression")
	ErrInvalidTimezone         = errors.New("invalid timezone")
	ErrScheduleUnauthorized    = errors.New("unauthorized to manage this schedule")
)

// CreateSchedule creates a new schedule for a plugin
func (s *Service) CreateSchedule(pluginID uint, scheduleID, name, cronExpr, timezone string, timeoutSeconds int, config map[string]interface{}, enabled bool) (*models.PluginSchedule, error) {
	// Verify plugin exists
	var plugin models.Plugin
	if err := s.DB.First(&plugin, pluginID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("plugin not found")
		}
		return nil, fmt.Errorf("failed to load plugin: %w", err)
	}

	// Check if schedule already exists
	var existing models.PluginSchedule
	result := s.DB.Where("plugin_id = ? AND manifest_schedule_id = ?", pluginID, scheduleID).First(&existing)
	if result.Error == nil {
		return nil, ErrScheduleAlreadyExists
	}

	// Set defaults
	if timezone == "" {
		timezone = "UTC"
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 60
	}

	// Convert config to JSON
	configJSON := "{}"
	if len(config) > 0 {
		if configBytes, err := json.Marshal(config); err == nil {
			configJSON = string(configBytes)
		}
	}

	// Create schedule
	schedule := &models.PluginSchedule{
		PluginID:           pluginID,
		ManifestScheduleID: scheduleID,
		Name:               name,
		CronExpr:           cronExpr,
		Timezone:           timezone,
		Enabled:            enabled,
		TimeoutSeconds:     timeoutSeconds,
		Config:             configJSON,
	}

	if err := s.DB.Create(schedule).Error; err != nil {
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}

	return schedule, nil
}

// GetSchedule retrieves a specific schedule
func (s *Service) GetSchedule(pluginID uint, scheduleID uint) (*models.PluginSchedule, error) {
	var schedule models.PluginSchedule
	if err := s.DB.Where("id = ? AND plugin_id = ?", scheduleID, pluginID).Preload("Plugin").First(&schedule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, fmt.Errorf("failed to load schedule: %w", err)
	}

	return &schedule, nil
}

// GetScheduleByManifestID retrieves a schedule by its manifest schedule ID
func (s *Service) GetScheduleByManifestID(pluginID uint, manifestScheduleID string) (*models.PluginSchedule, error) {
	var schedule models.PluginSchedule
	if err := s.DB.Where("plugin_id = ? AND manifest_schedule_id = ?", pluginID, manifestScheduleID).First(&schedule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, fmt.Errorf("failed to load schedule: %w", err)
	}

	return &schedule, nil
}

// ListSchedules lists all schedules for a plugin
func (s *Service) ListSchedules(pluginID uint) ([]models.PluginSchedule, error) {
	var schedules []models.PluginSchedule
	if err := s.DB.Where("plugin_id = ?", pluginID).Order("id ASC").Find(&schedules).Error; err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	return schedules, nil
}

// UpdateSchedule updates an existing schedule
func (s *Service) UpdateSchedule(pluginID uint, scheduleID uint, updates map[string]interface{}) (*models.PluginSchedule, error) {
	// Get existing schedule
	schedule, err := s.GetSchedule(pluginID, scheduleID)
	if err != nil {
		return nil, err
	}

	// Update fields
	if err := s.DB.Model(schedule).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}

	// Reload to get updated data
	if err := s.DB.First(schedule, scheduleID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload schedule: %w", err)
	}

	return schedule, nil
}

// DeleteSchedule deletes a schedule
func (s *Service) DeleteSchedule(pluginID uint, scheduleID uint) error {
	// Verify schedule exists and belongs to plugin
	schedule, err := s.GetSchedule(pluginID, scheduleID)
	if err != nil {
		return err
	}

	// Delete schedule (CASCADE will delete executions)
	if err := s.DB.Delete(schedule).Error; err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}

	return nil
}

// GetScheduleExecutions retrieves execution history for a schedule
func (s *Service) GetScheduleExecutions(pluginID uint, scheduleID uint, limit, offset int) ([]models.PluginScheduleExecution, int64, int64, float64, error) {
	// Verify schedule exists
	if _, err := s.GetSchedule(pluginID, scheduleID); err != nil {
		return nil, 0, 0, 0, err
	}

	var executions []models.PluginScheduleExecution
	var totalCount int64

	// Get total count
	s.DB.Model(&models.PluginScheduleExecution{}).Where("plugin_schedule_id = ?", scheduleID).Count(&totalCount)

	// Get executions with pagination
	if err := s.DB.
		Where("plugin_schedule_id = ?", scheduleID).
		Order("started_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&executions).Error; err != nil {
		return nil, 0, 0, 0, fmt.Errorf("failed to load executions: %w", err)
	}

	// Calculate success rate
	var successCount int64
	s.DB.Model(&models.PluginScheduleExecution{}).Where("plugin_schedule_id = ? AND success = ?", scheduleID, true).Count(&successCount)

	successRate := 0.0
	if totalCount > 0 {
		successRate = float64(successCount) / float64(totalCount) * 100
	}

	return executions, totalCount, successCount, successRate, nil
}
