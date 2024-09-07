package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateLLMSettings creates a new LLMSettings
func (s *Service) CreateLLMSettings(settings *models.LLMSettings) (*models.LLMSettings, error) {
	if err := settings.Create(s.DB); err != nil {
		return nil, err
	}
	return settings, nil
}

// GetLLMSettingsByID retrieves an LLMSettings by its ID
func (s *Service) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	settings := models.NewLLMSettings()
	if err := settings.Get(s.DB, id); err != nil {
		return nil, err
	}
	return settings, nil
}

// UpdateLLMSettings updates an existing LLMSettings
func (s *Service) UpdateLLMSettings(settings *models.LLMSettings) (*models.LLMSettings, error) {
	if err := settings.Update(s.DB); err != nil {
		return nil, err
	}
	return settings, nil
}

// DeleteLLMSettings deletes an LLMSettings by its ID
func (s *Service) DeleteLLMSettings(id uint) error {
	settings, err := s.GetLLMSettingsByID(id)
	if err != nil {
		return err
	}
	return settings.Delete(s.DB)
}

// GetAllLLMSettings retrieves all LLMSettings
func (s *Service) GetAllLLMSettings() (*models.LLMSettingsSlice, error) {
	var settings models.LLMSettingsSlice
	if err := settings.GetAll(s.DB); err != nil {
		return nil, err
	}
	return &settings, nil
}

// GetLLMSettingsByModel retrieves LLMSettings by model name
func (s *Service) GetLLMSettingsByModel(model string) (*models.LLMSettings, error) {
	settings := models.NewLLMSettings()
	if err := settings.GetByModel(s.DB, model); err != nil {
		return nil, err
	}
	return settings, nil
}

// SearchLLMSettingsByModelStub searches for LLMSettings by model name stub
func (s *Service) SearchLLMSettingsByModelStub(modelStub string) (*models.LLMSettingsSlice, error) {
	var settings models.LLMSettingsSlice
	if err := settings.SearchByModelStub(s.DB, modelStub); err != nil {
		return nil, err
	}
	return &settings, nil
}
