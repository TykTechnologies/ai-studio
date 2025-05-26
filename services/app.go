package services

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

var ERRPrivacyScoreMismatch = errors.New("Datasources have higher privacy requirements than the selected LLMs")

// CreateApp creates a new app with validity checks
func (s *Service) CreateApp(name, description string, userID uint, datasourceIDsStrings, llmIDsStrings, toolIDsStrings []string, monthlyBudget *float64, budgetStartDate *time.Time) (*models.App, error) {
	datasourceIDs, err := s.convertIDs(datasourceIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid datasource IDs: %w", err)
	}
	llmIDs, err := s.convertIDs(llmIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM IDs: %w", err)
	}
	toolIDs, err := s.convertIDs(toolIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid Tool IDs: %w", err)
	}

	// Check if datasources have higher privacy score than LLMs
	if err := s.validatePrivacyScores(datasourceIDs, llmIDs); err != nil {
		return nil, err
	}

	app := &models.App{
		Name:            name,
		Description:     description,
		UserID:          userID,
		MonthlyBudget:   monthlyBudget,
		BudgetStartDate: budgetStartDate,
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

	// Add Tools to the app
	for _, toolID := range toolIDs {
		tool, err := s.GetToolByID(toolID) // Assuming GetToolByID exists
		if err != nil {
			return nil, fmt.Errorf("failed to get tool %d: %w", toolID, err)
		}
		if err := app.AddTool(s.DB, tool); err != nil {
			return nil, fmt.Errorf("failed to add tool %d to app: %w", toolID, err)
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
	// Reload app to get all associations
	err = app.Get(s.DB, app.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload app: %w", err)
	}
	return app, nil
}

// UpdateApp updates an existing app with validity checks
func (s *Service) UpdateApp(id uint, name, description string, userID uint, datasourceIDsStrings, llmIDsStrings, toolIDsStrings []string, monthlyBudget *float64, budgetStartDate *time.Time) (*models.App, error) {
	app, err := s.GetAppByID(id)
	if err != nil {
		return nil, err
	}

	datasourceIDs, err := s.convertIDs(datasourceIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid datasource IDs: %w", err)
	}
	llmIDs, err := s.convertIDs(llmIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM IDs: %w", err)
	}
	toolIDs, err := s.convertIDs(toolIDsStrings)
	if err != nil {
		return nil, fmt.Errorf("invalid Tool IDs: %w", err)
	}

	// Check if datasources have higher privacy score than LLMs
	if err := s.validatePrivacyScores(datasourceIDs, llmIDs); err != nil {
		return nil, err
	}

	app.Name = name
	app.Description = description
	app.UserID = userID
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

	// Update Tools
	if err := s.updateAppTools(app, toolIDs); err != nil {
		return nil, fmt.Errorf("failed to update app tools: %w", err)
	}

	if err := app.Update(s.DB); err != nil {
		return nil, err
	}
	// Reload app to get all associations
	err = app.Get(s.DB, app.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload app: %w", err)
	}
	return app, nil
}

// Helper function to convert string IDs to uint IDs
func (s *Service) convertIDs(idStrings []string) ([]uint, error) {
	if idStrings == nil {
		return []uint{}, nil // Return empty slice if input is nil
	}
	ids := make([]uint, len(idStrings))
	for i, idStr := range idStrings {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return nil, err
		}
		ids[i] = uint(id)
	}
	return ids, nil
}

// validatePrivacyScores checks if any datasource has a higher privacy score than any LLM
func (s *Service) validatePrivacyScores(datasourceIDs, llmIDs []uint) error {
	var maxLLMScore int = -1 // Initialize with a value lower than any possible score
	var maxDatasourceScore int = -1 // Initialize with a value lower than any possible score

	if len(llmIDs) == 0 && len(datasourceIDs) == 0 {
		return nil
	}
	if len(llmIDs) == 0 && len(datasourceIDs) > 0 { // If only datasources are present, LLM score is effectively 0
		maxLLMScore = 0
	}
	if len(datasourceIDs) == 0 && len(llmIDs) > 0 { // If only llms are present, datasource score is effectively 0
		maxDatasourceScore = 0
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
	
	// Only enforce if both types of entities are present or if one type is present and the other is not (implicit score of 0)
	if maxDatasourceScore > -1 && maxLLMScore > -1 && maxDatasourceScore > maxLLMScore {
		return ERRPrivacyScoreMismatch
	}


	return nil
}

// updateAppDatasources updates the datasources associated with an app
func (s *Service) updateAppDatasources(app *models.App, datasourceIDs []uint) error {
	// Load existing datasources to ensure we have the current state
	s.DB.Model(app).Association("Datasources").Find(&app.Datasources)

	// Create a map of new datasource IDs for efficient lookup
	newDatasourceIDsMap := make(map[uint]bool)
	for _, dsID := range datasourceIDs {
		newDatasourceIDsMap[dsID] = true
	}

	// Remove datasources that are no longer in the list
	var datasourcesToKeep []models.Datasource
	for _, existingDS := range app.Datasources {
		if _, found := newDatasourceIDsMap[existingDS.ID]; found {
			datasourcesToKeep = append(datasourcesToKeep, existingDS)
			delete(newDatasourceIDsMap, existingDS.ID) // Remove from map as it's already associated
		}
	}
	app.Datasources = datasourcesToKeep

	// Add new datasources that were not previously associated
	for dsID := range newDatasourceIDsMap {
		ds, err := s.GetDatasourceByID(dsID)
		if err != nil {
			return fmt.Errorf("failed to get datasource %d: %w", dsID, err)
		}
		app.Datasources = append(app.Datasources, *ds)
	}
	
	return s.DB.Model(app).Association("Datasources").Replace(app.Datasources)
}

// updateAppLLMs updates the LLMs associated with an app
func (s *Service) updateAppLLMs(app *models.App, llmIDs []uint) error {
	s.DB.Model(app).Association("LLMs").Find(&app.LLMs)
	newLLMIDsMap := make(map[uint]bool)
	for _, llmID := range llmIDs {
		newLLMIDsMap[llmID] = true
	}

	var llmsToKeep []models.LLM
	for _, existingLLM := range app.LLMs {
		if _, found := newLLMIDsMap[existingLLM.ID]; found {
			llmsToKeep = append(llmsToKeep, existingLLM)
			delete(newLLMIDsMap, existingLLM.ID)
		}
	}
	app.LLMs = llmsToKeep

	for llmID := range newLLMIDsMap {
		llm, err := s.GetLLMByID(llmID)
		if err != nil {
			return fmt.Errorf("failed to get llm %d: %w", llmID, err)
		}
		app.LLMs = append(app.LLMs, *llm)
	}
	return s.DB.Model(app).Association("LLMs").Replace(app.LLMs)
}

// updateAppTools updates the Tools associated with an app
func (s *Service) updateAppTools(app *models.App, toolIDs []uint) error {
	s.DB.Model(app).Association("Tools").Find(&app.Tools) // Load existing tools
	newToolIDsMap := make(map[uint]bool)
	for _, toolID := range toolIDs {
		newToolIDsMap[toolID] = true
	}

	var toolsToKeep []models.Tool
	for _, existingTool := range app.Tools {
		if _, found := newToolIDsMap[existingTool.ID]; found {
			toolsToKeep = append(toolsToKeep, existingTool)
			delete(newToolIDsMap, existingTool.ID) // Remove from map
		}
	}
	app.Tools = toolsToKeep // Assign tools to keep

	// Add new tools
	for toolID := range newToolIDsMap {
		tool, err := s.GetToolByID(toolID) // Assuming GetToolByID exists
		if err != nil {
			return fmt.Errorf("failed to get tool %d: %w", toolID, err)
		}
		app.Tools = append(app.Tools, *tool)
	}
	// Replace the association with the final list of tools
	return s.DB.Model(app).Association("Tools").Replace(app.Tools)
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
	// Clear associations before deleting the app
	if err := s.DB.Model(app).Association("Datasources").Clear(); err != nil {
		return fmt.Errorf("failed to clear app datasources association: %w", err)
	}
	if err := s.DB.Model(app).Association("LLMs").Clear(); err != nil {
		return fmt.Errorf("failed to clear app LLMs association: %w", err)
	}
	if err := s.DB.Model(app).Association("Tools").Clear(); err != nil {
		return fmt.Errorf("failed to clear app tools association: %w", err)
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

// AddToolToApp adds a tool to an app
func (s *Service) AddToolToApp(appID, toolID uint) (*models.App, error) {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app %d: %w", appID, err)
	}

	tool, err := s.GetToolByID(toolID) // Assuming GetToolByID exists
	if err != nil {
		return nil, fmt.Errorf("failed to get tool %d: %w", toolID, err)
	}

	// Check if the tool is already associated to prevent duplicates if necessary,
	// though GORM's Append usually handles this for many2many.
	// For explicit control or error messaging:
	// for _, t := range app.Tools {
	// 	if t.ID == toolID {
	// 		return app, errors.New("tool already associated with this app")
	// 	}
	// }

	if err := app.AddTool(s.DB, tool); err != nil {
		return nil, fmt.Errorf("failed to add tool %d to app %d: %w", toolID, appID, err)
	}
	// Reload app to get all associations, including the newly added tool.
	err = app.Get(s.DB, app.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload app %d: %w", appID, err)
	}
	return app, nil
}

// RemoveToolFromApp removes a tool from an app
func (s *Service) RemoveToolFromApp(appID, toolID uint) error {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return fmt.Errorf("failed to get app %d: %w", appID, err)
	}

	tool, err := s.GetToolByID(toolID) // Assuming GetToolByID exists
	if err != nil {
		return fmt.Errorf("failed to get tool %d: %w", toolID, err)
	}

	if err := app.RemoveTool(s.DB, tool); err != nil {
		// Check if the error is because the association does not exist
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "record not found" { // GORM might return different error types/messages
			return fmt.Errorf("tool %d not associated with app %d or not found: %w", toolID, appID, err)
		}
		return fmt.Errorf("failed to remove tool %d from app %d: %w", toolID, appID, err)
	}
	return nil
}

// GetAppTools retrieves all tools associated with an app
func (s *Service) GetAppTools(appID uint) ([]models.Tool, error) {
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app %d: %w", appID, err)
	}

	// The GetTools method was added to models.App
	tools, err := app.GetTools(s.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to get tools for app %d: %w", appID, err)
	}
	return tools, nil
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

// GetToolByID retrieves a tool by its ID - Placeholder, ensure this exists or is implemented in tool_service.go
func (s *Service) GetToolByID(id uint) (*models.Tool, error) {
	tool := models.NewTool()
	// Assuming a Get method similar to other services
	if err := tool.Get(s.DB, id); err != nil {
		return nil, err
	}
	return tool, nil
}
