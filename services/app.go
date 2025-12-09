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
func (s *Service) CreateApp(name, description string, userID uint, datasourceIDs []uint, llmIDs []uint, toolIDs []uint, monthlyBudget *float64, budgetStartDate *time.Time, metadata map[string]interface{}) (*models.App, error) {
	// toolIDs is already of type []uint, no conversion needed

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
		Namespace:       "", // Default to global namespace
		Metadata:        metadata,
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
	// err = app.Get(s.DB, app.ID)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to reload app: %w", err)
	// }
	// return app, nil

	// Fetch into a new instance before returning to ensure all associations are freshly loaded
	finalApp := &models.App{}
	if err := finalApp.Get(s.DB, app.ID); err != nil {
		return nil, fmt.Errorf("failed to fetch final app state for app ID %d: %w", app.ID, err)
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppCreated(finalApp, finalApp.ID, userID)
	}

	return finalApp, nil
}

// CreateAppWithNamespace creates a new app with namespace support
func (s *Service) CreateAppWithNamespace(name, description string, userID uint, datasourceIDs []uint, llmIDs []uint, toolIDs []uint, monthlyBudget *float64, budgetStartDate *time.Time, namespace string, metadata map[string]interface{}) (*models.App, error) {
	// toolIDs is already of type []uint, no conversion needed

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
		Namespace:       namespace,
		Metadata:        metadata,
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
		tool, err := s.GetToolByID(toolID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool %d: %w", toolID, err)
		}
		if err := app.AddTool(s.DB, tool); err != nil {
			return nil, fmt.Errorf("failed to add tool %d to app: %w", toolID, err)
		}
	}

	// Fetch into a new instance before returning to ensure all associations are freshly loaded
	finalApp := &models.App{}
	if err := finalApp.Get(s.DB, app.ID); err != nil {
		return nil, fmt.Errorf("failed to fetch final app state for app ID %d: %w", app.ID, err)
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppCreated(finalApp, finalApp.ID, userID)
	}

	return finalApp, nil
}

// UpdateApp updates an existing app with validity checks
func (s *Service) UpdateApp(id uint, name, description string, userID uint, datasourceIDs []uint, llmIDs []uint, toolIDs []uint, monthlyBudget *float64, budgetStartDate *time.Time, metadata map[string]interface{}) (*models.App, error) {
	app, err := s.GetAppByID(id)
	if err != nil {
		return nil, err
	}

	// toolIDs is already of type []uint, no conversion needed

	// Check if datasources have higher privacy score than LLMs
	if err := s.validatePrivacyScores(datasourceIDs, llmIDs); err != nil {
		return nil, err
	}

	app.Name = name
	app.Description = description
	app.UserID = userID
	app.MonthlyBudget = monthlyBudget
	app.BudgetStartDate = budgetStartDate
	app.Metadata = metadata

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

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(app, app.ID, userID)
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
	var maxLLMScore int = -1        // Default to -1 if no LLMs
	var maxDatasourceScore int = -1 // Default to -1 if no datasources

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

	// Check if datasources have higher privacy score than LLMs
	if maxDatasourceScore > maxLLMScore {
		return ERRPrivacyScoreMismatch
	}

	return nil
}

// updateAppDatasources updates the datasources associated with an app
func (s *Service) updateAppDatasources(app *models.App, datasourceIDs []uint) error {
	// Load existing datasources
	s.DB.Model(app).Association("Datasources").Find(&app.Datasources)

	// Remove all existing datasources
	for _, ds := range app.Datasources {
		if err := app.RemoveDatasource(s.DB, &ds); err != nil {
			return err
		}
	}

	// Add all new datasources
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
	// Load existing LLMs
	s.DB.Model(app).Association("LLMs").Find(&app.LLMs)

	// Remove all existing LLMs
	for _, llm := range app.LLMs {
		if err := app.RemoveLLM(s.DB, &llm); err != nil {
			return err
		}
	}

	// Add all new LLMs
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

// updateAppTools updates the Tools associated with an app
func (s *Service) updateAppTools(app *models.App, toolIDs []uint) error {
	// Load existing Tools
	s.DB.Model(app).Association("Tools").Find(&app.Tools)

	// Remove all existing Tools
	for _, tool := range app.Tools {
		if err := app.RemoveTool(s.DB, &tool); err != nil {
			return err
		}
	}

	// Add all new Tools
	for _, toolID := range toolIDs {
		tool, err := s.GetToolByID(toolID)
		if err != nil {
			return err
		}
		if err := app.AddTool(s.DB, tool); err != nil {
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

	if err := app.Delete(s.DB); err != nil {
		return err
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppDeleted(id, 0)
	}

	return nil
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

	if err := app.ActivateCredential(s.DB); err != nil {
		return err
	}

	// Emit app approved event (credential activated)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppApproved(app, app.ID, 0)
	}

	return nil
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

// ListAppsWithFilters returns a paginated list of apps with namespace and active status filtering
func (s *Service) ListAppsWithFilters(pageSize, pageNumber int, all bool, sort, namespace string, isActive *bool) (models.Apps, int64, int, error) {
	var apps models.Apps
	totalCount, totalPages, err := apps.ListWithFilters(s.DB, pageSize, pageNumber, all, sort, namespace, isActive)
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

// GetAppsInNamespace returns apps in a specific namespace (including global)
func (s *Service) GetAppsInNamespace(namespace string) ([]models.App, error) {
	var apps []models.App
	
	query := s.DB.Preload("Credential").Preload("Datasources").Preload("LLMs").Preload("Tools")
	if namespace == "" {
		// Global namespace - only global apps
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := query.Find(&apps).Error; err != nil {
		return nil, err
	}

	return apps, nil
}

// GetActiveAppsInNamespace returns active apps in a specific namespace (including global)
func (s *Service) GetActiveAppsInNamespace(namespace string) ([]models.App, error) {
	var apps []models.App
	
	query := s.DB.Preload("Credential").Preload("Datasources").Preload("LLMs").Preload("Tools").Where("is_active = ?", true)
	if namespace == "" {
		// Global namespace - only global apps
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := query.Find(&apps).Error; err != nil {
		return nil, err
	}

	return apps, nil
}
