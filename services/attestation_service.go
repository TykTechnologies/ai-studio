package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateAttestationTemplate creates a new attestation template (admin)
func (s *Service) CreateAttestationTemplate(name, text, appliesToType string, required, active bool, sortOrder int) (*models.AttestationTemplate, error) {
	if appliesToType != models.AttestationAppliesToDatasource &&
		appliesToType != models.AttestationAppliesToTool &&
		appliesToType != models.AttestationAppliesToAll {
		return nil, fmt.Errorf("invalid applies_to_type: must be '%s', '%s', or '%s'",
			models.AttestationAppliesToDatasource, models.AttestationAppliesToTool, models.AttestationAppliesToAll)
	}

	template := &models.AttestationTemplate{
		Name:          name,
		Text:          text,
		Required:      required,
		AppliesToType: appliesToType,
		Active:        active,
		SortOrder:     sortOrder,
	}

	if err := template.Create(s.DB); err != nil {
		return nil, err
	}
	return template, nil
}

// GetAttestationTemplateByID retrieves an attestation template by ID
func (s *Service) GetAttestationTemplateByID(id uint) (*models.AttestationTemplate, error) {
	template := models.NewAttestationTemplate()
	if err := template.Get(s.DB, id); err != nil {
		return nil, err
	}
	return template, nil
}

// UpdateAttestationTemplate updates an attestation template (admin)
func (s *Service) UpdateAttestationTemplate(id uint, name, text, appliesToType string, required, active bool, sortOrder int) (*models.AttestationTemplate, error) {
	template, err := s.GetAttestationTemplateByID(id)
	if err != nil {
		return nil, err
	}

	if appliesToType != models.AttestationAppliesToDatasource &&
		appliesToType != models.AttestationAppliesToTool &&
		appliesToType != models.AttestationAppliesToAll {
		return nil, fmt.Errorf("invalid applies_to_type: must be '%s', '%s', or '%s'",
			models.AttestationAppliesToDatasource, models.AttestationAppliesToTool, models.AttestationAppliesToAll)
	}

	template.Name = name
	template.Text = text
	template.Required = required
	template.AppliesToType = appliesToType
	template.Active = active
	template.SortOrder = sortOrder

	if err := template.Update(s.DB); err != nil {
		return nil, err
	}
	return template, nil
}

// DeleteAttestationTemplate deletes an attestation template (admin)
func (s *Service) DeleteAttestationTemplate(id uint) error {
	template, err := s.GetAttestationTemplateByID(id)
	if err != nil {
		return err
	}
	return template.Delete(s.DB)
}

// GetAllAttestationTemplates retrieves all attestation templates
func (s *Service) GetAllAttestationTemplates(activeOnly bool) (models.AttestationTemplates, error) {
	var templates models.AttestationTemplates
	if err := templates.GetAll(s.DB, activeOnly); err != nil {
		return nil, err
	}
	return templates, nil
}

// GetAttestationTemplatesByType retrieves templates applicable to a resource type
func (s *Service) GetAttestationTemplatesByType(resourceType string, activeOnly bool) (models.AttestationTemplates, error) {
	var templates models.AttestationTemplates
	if err := templates.GetByType(s.DB, resourceType, activeOnly); err != nil {
		return nil, err
	}
	return templates, nil
}
