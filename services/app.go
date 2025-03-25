package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

var ERRPrivacyScoreMismatch = errors.New("Datasources have higher privacy score than LLMs")

// CreateApp creates a new app with validity checks
func (s *Service) CreateApp(name, description string, userID uint, datasourceIDs, llmIDs []uint) (*models.App, error) {
	// Check if datasources have higher privacy score than LLMs
	if err := s.validatePrivacyScores(datasourceIDs, llmIDs); err != nil {
		return nil, err
	}

	app := &models.App{
		Name:        name,
		Description: description,
		UserID:      userID,
	}

	if err := app.Create(s.DB); err != nil {
		return nil, err
	}

	// Add datasources to the app
	for _, dsID := range datasourceIDs {
		ds, err := s.GetDatasourceByID(dsID)
		if err != nil {
			return nil, err
		}
		if err := app.AddDatasource(s.DB, ds); err != nil {
			return nil, err
		}
	}

	// Add LLMs to the app
	for _, llmID := range llmIDs {
		llm, err := s.GetLLMByID(llmID)
		if err != nil {
			return nil, err
		}
		if err := app.AddLLM(s.DB, llm); err != nil {
			return nil, err
		}
	}

	// Get the user who created the app
	user, err := s.GetUserByID(userID)
	if err != nil {
		// Log error but don't fail app creation
		fmt.Printf("Error getting user for notification: %v\n", err)
	} else {
		// Send notification to admin users
		data := struct {
			AppName        string
			AppDescription string
			UserName       string
			AppDetailsURL  string
		}{
			AppName:        app.Name,
			AppDescription: app.Description,
			UserName:       user.Name,
			AppDetailsURL:  fmt.Sprintf("/admin/apps/%d", app.ID),
		}
		notificationID := fmt.Sprintf("new_app_%d_%d", app.ID, time.Now().UnixNano())
		if err := s.NotificationService.Notify(notificationID, "New App Created on AI Portal", "admin-app-notification.tmpl", data, models.NotifyAdmins); err != nil {
			// Ignore notification errors - they shouldn't fail app creation
			fmt.Printf("Error sending admin notification: %v\n", err)
		}
	}

	return app, nil
}

// UpdateApp updates an existing app with validity checks
func (s *Service) UpdateApp(id uint, name, description string, datasourceIDs, llmIDs []uint, monthlyBudget *float64, budgetStartDate *time.Time) (*models.App, error) {
	app, err := s.GetAppByID(id)
	if err != nil {
		return nil, err
	}

	// Check if datasources have higher privacy score than LLMs
	if err := s.validatePrivacyScores(datasourceIDs, llmIDs); err != nil {
		return nil, err
	}

	app.Name = name
	app.Description = description
	app.MonthlyBudget = monthlyBudget
	app.BudgetStartDate = budgetStartDate

	// Update datasources
	if err := s.updateAppDatasources(app, datasourceIDs); err != nil {
		return nil, err
	}

	// Update LLMs
	if err := s.updateAppLLMs(app, llmIDs); err != nil {
		return nil, err
	}

	if err := app.Update(s.DB); err != nil {
		return nil, err
	}

	return app, nil
}

// validatePrivacyScores checks if any datasource has a higher privacy score than any LLM
func (s *Service) validatePrivacyScores(datasourceIDs, llmIDs []uint) error {
	var maxLLMScore int
	var maxDatasourceScore int = 0 // Initialize with a value higher than the maximum possible score

	if len(llmIDs) == 0 && len(datasourceIDs) == 0 {
		return nil
	}

	for _, llmID := range llmIDs {
		llm, err := s.GetLLMByID(llmID)
		if err != nil {
			return err
		}
		if llm.PrivacyScore > maxLLMScore {
			maxLLMScore = llm.PrivacyScore
		}
	}

	for _, dsID := range datasourceIDs {
		ds, err := s.GetDatasourceByID(dsID)
		if err != nil {
			return err
		}
		if ds.PrivacyScore > maxDatasourceScore {
			maxDatasourceScore = ds.PrivacyScore
		}
	}

	if maxDatasourceScore > maxLLMScore {
		return ERRPrivacyScoreMismatch
	}

	return nil
}

// updateAppDatasources updates the datasources associated with an app
func (s *Service) updateAppDatasources(app *models.App, datasourceIDs []uint) error {
	// Remove existing datasources
	for _, ds := range app.Datasources {
		if err := app.RemoveDatasource(s.DB, &ds); err != nil {
			return err
		}
	}

	// Add new datasources
	for _, dsID := range datasourceIDs {
		ds, err := s.GetDatasourceByID(dsID)
		if err != nil {
			return err
		}
		if err := app.AddDatasource(s.DB, ds); err != nil {
			return err
		}
	}

	return nil
}

// updateAppLLMs updates the LLMs associated with an app
func (s *Service) updateAppLLMs(app *models.App, llmIDs []uint) error {
	// Remove existing LLMs
	for _, llm := range app.LLMs {
		if err := app.RemoveLLM(s.DB, &llm); err != nil {
			return err
		}
	}

	// Add new LLMs
	for _, llmID := range llmIDs {
		llm, err := s.GetLLMByID(llmID)
		if err != nil {
			return err
		}
		if err := app.AddLLM(s.DB, llm); err != nil {
			return err
		}
	}

	return nil
}

// GetAppByID retrieves an app by its ID
func (s *Service) GetAppByID(id uint) (*models.App, error) {
	app := models.NewApp()
	if err := app.Get(s.DB, id); err != nil {
		return nil, err
	}
	return app, nil
}

// GetAppByCredentialID retrieves an app by its credential ID
func (s *Service) GetAppByCredentialID(credentialID uint) (*models.App, error) {
	app := models.NewApp()
	if err := app.GetByCredentialID(s.DB, credentialID); err != nil {
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

	// Delete the associated credential
	if app.CredentialID != 0 {
		if err := s.DeleteCredential(app.CredentialID); err != nil {
			return err
		}
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

// AddDatasourceToApp adds a datasource to an app
func (s *Service) AddDatasourceToApp(appID, datasourceID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	datasource, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return err
	}

	return app.AddDatasource(s.DB, datasource)
}

// RemoveDatasourceFromApp removes a datasource from an app
func (s *Service) RemoveDatasourceFromApp(appID, datasourceID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	datasource, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return err
	}

	return app.RemoveDatasource(s.DB, datasource)
}

// GetAppDatasources retrieves all datasources associated with an app
func (s *Service) GetAppDatasources(appID uint) ([]models.Datasource, error) {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}

	if err := app.GetDatasources(s.DB); err != nil {
		return nil, err
	}

	return app.Datasources, nil
}

// AddLLMToApp adds an LLM to an app
func (s *Service) AddLLMToApp(appID, llmID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	llm, err := s.GetLLMByID(llmID)
	if err != nil {
		return err
	}

	return app.AddLLM(s.DB, llm)
}

// RemoveLLMFromApp removes an LLM from an app
func (s *Service) RemoveLLMFromApp(appID, llmID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return err
	}

	llm, err := s.GetLLMByID(llmID)
	if err != nil {
		return err
	}

	return app.RemoveLLM(s.DB, llm)
}

// GetAppLLMs retrieves all LLMs associated with an app
func (s *Service) GetAppLLMs(appID uint, pageSize int, pageNumber int, all bool) ([]models.LLM, int64, int, error) {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, 0, 0, err
	}

	llms, totalCount, totalPages, err := app.GetLLMs(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}

	return llms, totalCount, totalPages, nil
}

// ListApps returns all apps
func (s *Service) ListApps() (models.Apps, error) {
	app := models.NewApp()
	return app.List(s.DB)
}

// ListAppsWithPagination returns a paginated list of apps
func (s *Service) ListAppsWithPagination(pageSize, pageNumber int, all bool, sort string) (models.Apps, int64, int, error) {
	var apps models.Apps
	totalCount, totalPages, err := apps.ListWithPagination(s.DB, pageSize, pageNumber, all, sort)
	return apps, totalCount, totalPages, err
}

// ListAppsByUserID returns all apps for a specific user with pagination
func (s *Service) ListAppsByUserID(userID uint, pageSize, pageNumber int, all bool, sort string) (models.Apps, int64, int, error) {
	var apps models.Apps
	totalCount, totalPages, err := apps.ListByUserID(s.DB, userID, pageSize, pageNumber, all, sort)
	return apps, totalCount, totalPages, err
}

// SearchApps returns apps matching the given search term with pagination
func (s *Service) SearchApps(searchTerm string, pageSize, pageNumber int, all bool, sort string) (models.Apps, int64, int, error) {
	var apps models.Apps
	totalCount, totalPages, err := apps.Search(s.DB, searchTerm, pageSize, pageNumber, all, sort)
	return apps, totalCount, totalPages, err
}

// CountApps returns the total number of apps
func (s *Service) CountApps() (int64, error) {
	app := models.NewApp()
	return app.Count(s.DB)
}

// CountAppsByUserID returns the total number of apps for a specific user
func (s *Service) CountAppsByUserID(userID uint) (int64, error) {
	app := models.NewApp()
	return app.CountByUserID(s.DB, userID)
}
