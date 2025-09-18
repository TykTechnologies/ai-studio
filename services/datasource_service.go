package services

import (
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

	if err := datasource.Create(s.DB); err != nil {
		return nil, err
	}

	if err := datasource.AddTags(s.DB, tagNames); err != nil {
		return nil, err
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
	datasource.DBConnString = dbConnString
	datasource.DBSourceType = dbSourceType
	// Smart DB connection API key update logic
	if dbConnAPIKey == "[redacted]" {
		// Don't update API key if it's the redacted placeholder
	} else if dbConnAPIKey == "" {
		// Clear API key if empty string is provided
		datasource.DBConnAPIKey = ""
	} else {
		// Update to new API key value
		datasource.DBConnAPIKey = dbConnAPIKey
	}
	datasource.EmbedVendor = models.Vendor(embedVendor)
	datasource.EmbedUrl = embedUrl
	// Smart embed API key update logic
	if embedAPIKey == "[redacted]" {
		// Don't update API key if it's the redacted placeholder
	} else if embedAPIKey == "" {
		// Clear API key if empty string is provided
		datasource.EmbedAPIKey = ""
	} else {
		// Update to new API key value
		datasource.EmbedAPIKey = embedAPIKey
	}
	datasource.EmbedModel = embedModel
	datasource.DBName = dbName
	datasource.Active = active
	datasource.UserID = userID

	oldTags := []string{}
	for _, tag := range datasource.Tags {
		oldTags = append(oldTags, tag.Name)
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

	return datasource.Delete(s.DB)
}

func (s *Service) GetAllDatasources(pageSize int, pageNumber int, all bool) (models.Datasources, int64, int, error) {
	var datasources models.Datasources
	totalCount, totalPages, err := datasources.GetAll(s.DB, pageSize, pageNumber, all)
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

// TODO:
// - StartProcessingFiles method (Starts RAG with DataSourceSession)
// - Make sure chats with default DS load them on init
