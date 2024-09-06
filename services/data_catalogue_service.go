package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateDataCatalogue(name, shortDesc, longDesc, icon string) (*models.DataCatalogue, error) {
	dataCatalogue := &models.DataCatalogue{
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Icon:             icon,
	}

	if err := dataCatalogue.Create(s.DB); err != nil {
		return nil, err
	}

	return dataCatalogue, nil
}

func (s *Service) GetDataCatalogueByID(id uint) (*models.DataCatalogue, error) {
	dataCatalogue := models.NewDataCatalogue()
	if err := dataCatalogue.Get(s.DB, id); err != nil {
		return nil, err
	}
	return dataCatalogue, nil
}

func (s *Service) UpdateDataCatalogue(id uint, name, shortDesc, longDesc, icon string) (*models.DataCatalogue, error) {
	dataCatalogue, err := s.GetDataCatalogueByID(id)
	if err != nil {
		return nil, err
	}

	dataCatalogue.Name = name
	dataCatalogue.ShortDescription = shortDesc
	dataCatalogue.LongDescription = longDesc
	dataCatalogue.Icon = icon

	if err := dataCatalogue.Update(s.DB); err != nil {
		return nil, err
	}

	return dataCatalogue, nil
}

func (s *Service) DeleteDataCatalogue(id uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(id)
	if err != nil {
		return err
	}

	return dataCatalogue.Delete(s.DB)
}

func (s *Service) AddTagToDataCatalogue(dataCatalogueID, tagID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	tag, err := s.GetTagByID(tagID)
	if err != nil {
		return err
	}

	return dataCatalogue.AddTag(s.DB, tag)
}

func (s *Service) RemoveTagFromDataCatalogue(dataCatalogueID, tagID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	tag, err := s.GetTagByID(tagID)
	if err != nil {
		return err
	}

	return dataCatalogue.RemoveTag(s.DB, tag)
}

func (s *Service) AddDatasourceToDataCatalogue(dataCatalogueID, datasourceID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	datasource, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return err
	}

	return dataCatalogue.AddDatasource(s.DB, datasource)
}

func (s *Service) RemoveDatasourceFromDataCatalogue(dataCatalogueID, datasourceID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	datasource, err := s.GetDatasourceByID(datasourceID)
	if err != nil {
		return err
	}

	return dataCatalogue.RemoveDatasource(s.DB, datasource)
}

func (s *Service) GetAllDataCatalogues() (models.DataCatalogues, error) {
	var dataCatalogues models.DataCatalogues
	if err := dataCatalogues.GetAll(s.DB); err != nil {
		return nil, err
	}
	return dataCatalogues, nil
}

func (s *Service) SearchDataCatalogues(query string) (models.DataCatalogues, error) {
	var dataCatalogues models.DataCatalogues
	if err := dataCatalogues.Search(s.DB, query); err != nil {
		return nil, err
	}
	return dataCatalogues, nil
}

func (s *Service) GetDataCataloguesByTag(tagName string) (models.DataCatalogues, error) {
	var dataCatalogues models.DataCatalogues
	if err := dataCatalogues.GetByTag(s.DB, tagName); err != nil {
		return nil, err
	}
	return dataCatalogues, nil
}

func (s *Service) GetDataCataloguesByDatasource(datasourceID uint) (models.DataCatalogues, error) {
	var dataCatalogues models.DataCatalogues
	if err := dataCatalogues.GetByDatasource(s.DB, datasourceID); err != nil {
		return nil, err
	}
	return dataCatalogues, nil
}
