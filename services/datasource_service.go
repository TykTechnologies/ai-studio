package services

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func (s *Service) CreateDatasource(name, shortDesc, longDesc, icon, url string, privacyScore int, userID uint, tagNames []string, dbConnString, dbSourceType, dbConnAPIKey, dbName, embedVendor, embedUrl, embedAPIKey, embedModel string, active bool) (*models.Datasource, error) {
	datasource := &models.Datasource{
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Icon:             icon,
		Url:              url,
		PrivacyScore:     privacyScore,
		UserID:           userID,
		DBConnString:     dbConnString,
		DBSourceType:     dbSourceType,
		DBConnAPIKey:     dbConnAPIKey,
		DBName:           dbName,
		EmbedVendor:      models.Vendor(embedVendor),
		EmbedUrl:         embedUrl,
		EmbedAPIKey:      embedAPIKey,
		EmbedModel:       embedModel,
		Active:           active,
	}

	// Execute "before_create" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookBeforeCreate,
			datasource,
			uint32(userID),
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.Datasource); ok {
				datasource = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(datasource, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	if err := datasource.Create(s.DB); err != nil {
		return nil, err
	}

	if err := datasource.AddTags(s.DB, tagNames); err != nil {
		return nil, err
	}

	// Auto-assign to Default data catalogue if not in any catalogue
	if err := s.ensureDatasourceInDefaultCatalogue(datasource); err != nil {
		// Log but don't fail - this is a convenience feature
		logger.Warn(fmt.Sprintf("Failed to add datasource to default catalogue: %v", err))
	}

	// Execute "after_create" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookAfterCreate,
			datasource,
			uint32(userID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-create hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitDatasourceCreated(datasource, datasource.ID, userID)
	}

	return datasource, nil
}

func (s *Service) UpdateDatasource(id uint, name, shortDesc, longDesc, icon, url string, privacyScore int, dbConnString, dbSourceType, dbConnAPIKey, dbName, embedVendor, embedUrl, embedAPIKey, embedModel string, active bool, tagNames []string, userID uint) (*models.Datasource, error) {
	datasource, err := s.GetDatasourceByID(id)
	if err != nil {
		return nil, err
	}

	datasource.Name = name
	datasource.ShortDescription = shortDesc
	datasource.LongDescription = longDesc
	datasource.Icon = icon
	datasource.Url = url
	datasource.PrivacyScore = privacyScore

	// Smart connection string update - preserve if empty
	if dbConnString != "" {
		datasource.DBConnString = dbConnString
	}
	if dbSourceType != "" {
		datasource.DBSourceType = dbSourceType
	}

	// Smart DB connection API key update logic
	if dbConnAPIKey == "[redacted]" {
		// Don't update API key if it's the redacted placeholder
	} else if dbConnAPIKey == "" {
		// Empty = preserve existing (don't clear)
	} else {
		// Update to new API key value
		datasource.DBConnAPIKey = dbConnAPIKey
	}

	if embedVendor != "" {
		datasource.EmbedVendor = models.Vendor(embedVendor)
	}

	// Smart embed URL update - preserve if empty
	if embedUrl != "" {
		datasource.EmbedUrl = embedUrl
	}

	// Smart embed API key update logic
	if embedAPIKey == "[redacted]" {
		// Don't update API key if it's the redacted placeholder
	} else if embedAPIKey == "" {
		// Empty = preserve existing (don't clear)
	} else {
		// Update to new API key value
		datasource.EmbedAPIKey = embedAPIKey
	}

	if embedModel != "" {
		datasource.EmbedModel = embedModel
	}
	datasource.DBName = dbName
	datasource.Active = active
	datasource.UserID = userID

	oldTags := []string{}
	for _, tag := range datasource.Tags {
		oldTags = append(oldTags, tag.Name)
	}

	// Execute "before_update" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookBeforeUpdate,
			datasource,
			uint32(userID),
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.Datasource); ok {
				datasource = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(datasource, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	if err := datasource.Update(s.DB); err != nil {
		return nil, err
	}

	if err := datasource.RemoveTags(s.DB, oldTags); err != nil {
		return nil, err
	}

	if err := datasource.AddTags(s.DB, tagNames); err != nil {
		return nil, err
	}

	// Execute "after_update" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookAfterUpdate,
			datasource,
			uint32(userID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-update hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitDatasourceUpdated(datasource, datasource.ID, userID)
	}

	return datasource, nil
}

func (s *Service) GetDatasourceByID(id uint) (*models.Datasource, error) {
	datasource := models.NewDatasource()
	if err := datasource.Get(s.DB, id); err != nil {
		return nil, err
	}

	datasource.DBConnAPIKey = secrets.GetValue(datasource.DBConnAPIKey, true) // preserve reference for API responses
	datasource.EmbedAPIKey = secrets.GetValue(datasource.EmbedAPIKey, true)   // preserve reference for API responses
	return datasource, nil
}

func (s *Service) DeleteDatasource(id uint) error {
	datasource, err := s.GetDatasourceByID(id)
	if err != nil {
		return err
	}

	// Execute "before_delete" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookBeforeDelete,
			datasource,
			uint32(datasource.UserID),
		)
		if err != nil {
			return fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}
	}

	if err := datasource.Delete(s.DB); err != nil {
		return err
	}

	// Execute "after_delete" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeDatasource,
			HookAfterDelete,
			datasource,
			uint32(datasource.UserID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-delete hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitDatasourceDeleted(id, datasource.UserID)
	}

	return nil
}

// CloneDatasource creates a copy of an existing datasource with all configuration including API keys
func (s *Service) CloneDatasource(sourceDatasourceID uint) (*models.Datasource, error) {
	// Get source datasource with all relationships
	source, err := s.GetDatasourceByID(sourceDatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source datasource: %w", err)
	}

	// Extract tag names for re-association
	tagNames := make([]string, len(source.Tags))
	for i, tag := range source.Tags {
		tagNames[i] = tag.Name
	}

	// Create cloned datasource with "Copy of" prefix and inactive status
	// IMPORTANT: This preserves API keys (DBConnAPIKey, EmbedAPIKey)
	cloned, err := s.CreateDatasource(
		fmt.Sprintf("Copy of %s", source.Name), // New name
		source.ShortDescription,
		source.LongDescription,
		source.Icon,
		source.Url,
		source.PrivacyScore,
		source.UserID,
		tagNames,
		source.DBConnString,
		source.DBSourceType,
		source.DBConnAPIKey, // API key preserved
		source.DBName,
		string(source.EmbedVendor), // Convert Vendor to string
		source.EmbedUrl,
		source.EmbedAPIKey, // API key preserved
		source.EmbedModel,
		false, // Start inactive for safety
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create cloned datasource: %w", err)
	}

	logger.Info(fmt.Sprintf("Datasource cloned successfully: source_id=%d, cloned_id=%d, cloned_name=%s",
		sourceDatasourceID, cloned.ID, cloned.Name))

	return cloned, nil
}

func (s *Service) GetAllDatasources(pageSize int, pageNumber int, all bool) (models.Datasources, int64, int, error) {
	var datasources models.Datasources
	totalCount, totalPages, err := datasources.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return datasources, totalCount, totalPages, nil
}

// GetAllDatasourcesWithFilters returns all datasources with filtering by active status and user ID
func (s *Service) GetAllDatasourcesWithFilters(pageSize int, pageNumber int, all bool, isActive *bool, userID *uint) (models.Datasources, int64, int, error) {
	var datasources models.Datasources
	totalCount, totalPages, err := datasources.GetAllWithFilters(s.DB, pageSize, pageNumber, all, isActive, userID)
	if err != nil {
		return nil, 0, 0, err
	}
	return datasources, totalCount, totalPages, nil
}

func (s *Service) GetActiveDatasources() ([]models.Datasource, error) {
	var datasources models.Datasources
	if err := datasources.GetActiveDataSources(s.DB); err != nil {
		return nil, err
	}
	return []models.Datasource(datasources), nil
}

func (s *Service) SearchDatasources(query string) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.Search(s.DB, query); err != nil {
		return nil, err
	}

	for i := range datasources {
		datasources[i].DBConnAPIKey = secrets.GetValue(datasources[i].DBConnAPIKey, true) // preserve reference for API responses
		datasources[i].EmbedAPIKey = secrets.GetValue(datasources[i].EmbedAPIKey, true)   // preserve reference for API responses
	}

	return datasources, nil
}

func (s *Service) GetDatasourcesByTag(tagName string) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetByTag(s.DB, tagName); err != nil {
		return nil, err
	}
	return datasources, nil
}

func (s *Service) AddTagsToDatasource(datasourceID uint, tagNames []string) error {
	datasource, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return err
	}

	return datasource.AddTags(s.DB, tagNames)
}

func (s *Service) GetDatasourcesByMinPrivacyScore(minScore int) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetByMinPrivacyScore(s.DB, minScore); err != nil {
		return nil, err
	}
	return datasources, nil
}

func (s *Service) GetDatasourcesByMaxPrivacyScore(maxScore int) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetByMaxPrivacyScore(s.DB, maxScore); err != nil {
		return nil, err
	}
	return datasources, nil
}

func (s *Service) GetDatasourcesByPrivacyScoreRange(minScore, maxScore int) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetByPrivacyScoreRange(s.DB, minScore, maxScore); err != nil {
		return nil, err
	}
	return datasources, nil
}

func (s *Service) GetDatasourcesByUserID(userID uint) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetByUserID(s.DB, userID); err != nil {
		return nil, err
	}
	return datasources, nil
}

// AddFileStoreToTool adds a FileStore to a Tool
func (s *Service) AddFileToDatasource(dsID uint, fileStoreID uint) error {
	ds, err := s.GetDatasourceByID(dsID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return ds.AddFileStore(s.DB, fileStore)
}

// RemoveFileStoreFromTool removes a FileStore from a Tool
func (s *Service) RemoveFileFromDatasource(dsID uint, fileStoreID uint) error {
	ds, err := s.GetDatasourceByID(dsID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return ds.RemoveFileStore(s.DB, fileStore)
}

// ensureDatasourceInDefaultCatalogue adds a datasource to the Default data catalogue if it's not in any catalogue
func (s *Service) ensureDatasourceInDefaultCatalogue(datasource *models.Datasource) error {
	// Check if datasource is in any catalogue
	count := s.DB.Model(datasource).Association("DataCatalogues").Count()

	if count == 0 {
		// Get or create default data catalogue
		defaultCatalogue, err := models.GetOrCreateDefaultDataCatalogue(s.DB)
		if err != nil {
			return fmt.Errorf("failed to get default data catalogue: %w", err)
		}

		// Add datasource to default catalogue
		if err := s.DB.Model(defaultCatalogue).Association("Datasources").Append(datasource); err != nil {
			return fmt.Errorf("failed to add datasource to default catalogue: %w", err)
		}

		logger.Infof("Auto-assigned datasource '%s' (ID: %d) to Default data catalogue", datasource.Name, datasource.ID)
	}

	return nil
}

// TODO:
// - StartProcessingFiles method (Starts RAG with DataSourceSession)
// - Make sure chats with default DS load them on init
