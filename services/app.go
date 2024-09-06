package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateApp creates a new app
func (s *Service) CreateApp(name, description string, userID uint) (*models.App, error) {
	app := &models.App{
		Name:        name,
		Description: description,
		UserID:      userID,
	}

	if err := app.Create(s.DB); err != nil {
		return nil, err
	}

	return app, nil
}

// GetAppByID retrieves an app by its ID
func (s *Service) GetAppByID(id uint) (*models.App, error) {
	app := models.NewApp()
	if err := app.Get(s.DB, id); err != nil {
		return nil, err
	}
	return app, nil
}

// UpdateApp updates an existing app
func (s *Service) UpdateApp(id uint, name, description string) (*models.App, error) {
	app, err := s.GetAppByID(id)
	if err != nil {
		return nil, err
	}

	app.Name = name
	app.Description = description

	if err := app.Update(s.DB); err != nil {
		return nil, err
	}

	return app, nil
}

// DeleteApp deletes an app
func (s *Service) DeleteApp(id uint) error {
	app, err := s.GetAppByID(id)
	if err != nil {
		return err
	}

	return app.Delete(s.DB)
}

// GetAppsByUserID retrieves all apps for a specific user
func (s *Service) GetAppsByUserID(userID uint) ([]models.App, error) {
	app := models.NewApp()
	return app.GetByUserID(s.DB, userID)
}

// GetAppByName retrieves an app by its name
func (s *Service) GetAppByName(name string) (*models.App, error) {
	app := models.NewApp()
	if err := app.GetByName(s.DB, name); err != nil {
		return nil, err
	}
	return app, nil
}

// ActivateAppCredential activates the credential associated with an app
func (s *Service) ActivateAppCredential(appID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	return app.ActivateCredential(s.DB)
}

// DeactivateAppCredential deactivates the credential associated with an app
func (s *Service) DeactivateAppCredential(appID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	return app.DeactivateCredential(s.DB)
}
