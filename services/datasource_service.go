package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateDatasource(name, shortDesc, longDesc, icon, url string, privacyScore int, userID uint, tagNames []string, dbConnString, dbSourceType, dbConnAPIKey, dbName, embedVendor, embedUrl, embedAPIKey, embedModel string) (*models.Datasource, error) {
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
	}

	if err := datasource.Create(s.DB); err != nil {
		return nil, err
	}

	if err := datasource.AddTags(s.DB, tagNames); err != nil {
		return nil, err
	}

	return datasource, nil
}

func (s *Service) UpdateDatasource(id uint, name, shortDesc, longDesc, icon, url string, privacyScore int, dbConnString, dbSourceType, dbConnAPIKey, dbName, embedVendor, embedUrl, embedAPIKey, embedModel string) (*models.Datasource, error) {
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
	datasource.DBConnAPIKey = dbConnAPIKey
	datasource.EmbedVendor = models.Vendor(embedVendor)
	datasource.EmbedUrl = embedUrl
	datasource.EmbedAPIKey = embedAPIKey
	datasource.EmbedModel = embedModel
	datasource.DBName = dbName

	if err := datasource.Update(s.DB); err != nil {
		return nil, err
	}

	return datasource, nil
}

func (s *Service) GetDatasourceByID(id uint) (*models.Datasource, error) {
	datasource := models.NewDatasource()
	if err := datasource.Get(s.DB, id); err != nil {
		return nil, err
	}
	return datasource, nil
}

func (s *Service) DeleteDatasource(id uint) error {
	datasource, err := s.GetDatasourceByID(id)
	if err != nil {
		return err
	}

	return datasource.Delete(s.DB)
}

func (s *Service) GetAllDatasources() (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.GetAll(s.DB); err != nil {
		return nil, err
	}
	return datasources, nil
}

func (s *Service) SearchDatasources(query string) (models.Datasources, error) {
	var datasources models.Datasources
	if err := datasources.Search(s.DB, query); err != nil {
		return nil, err
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
