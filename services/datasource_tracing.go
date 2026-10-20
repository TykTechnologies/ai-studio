package services

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// SetDatasourceTracingContentModeMax sets a datasource's agent tracing
// content mode cap ("" removes it). CreateDatasource and UpdateDatasource
// take their attributes positionally, so the cap is written on its own:
// before UpdateDatasource (which reloads and saves the row, so its change
// event carries the cap) and after CreateDatasource (a new datasource is in
// no App yet, so no edge depends on it).
func (s *Service) SetDatasourceTracingContentModeMax(id uint, mode string) error {
	if err := models.ValidateTracingContentMode("tracing_content_mode_max", mode); err != nil {
		return helpers.NewBadRequestError(err.Error())
	}
	return s.DB.Model(&models.Datasource{}).Where("id = ?", id).Update("tracing_content_mode_max", mode).Error
}
