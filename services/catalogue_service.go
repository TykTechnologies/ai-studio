package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateCatalogue(name string) (*models.Catalogue, error) {
	catalogue := &models.Catalogue{
		Name: name,
	}

	if err := catalogue.Create(s.DB); err != nil {
		return nil, err
	}

	return catalogue, nil
}

func (s *Service) GetCatalogueByID(id uint) (*models.Catalogue, error) {
	catalogue := models.NewCatalogue()
	if err := catalogue.Get(s.DB, id); err != nil {
		return nil, err
	}
	return catalogue, nil
}

func (s *Service) UpdateCatalogue(id uint, name string) (*models.Catalogue, error) {
	catalogue, err := s.GetCatalogueByID(id)
	if err != nil {
		return nil, err
	}

	catalogue.Name = name

	if err := catalogue.Update(s.DB); err != nil {
		return nil, err
	}

	return catalogue, nil
}

func (s *Service) DeleteCatalogue(id uint) error {
	catalogue, err := s.GetCatalogueByID(id)
	if err != nil {
		return err
	}

	return catalogue.Delete(s.DB)
}

func (s *Service) AddLLMToCatalogue(llmID, catalogueID uint) error {
	llm, err := s.GetLLMByID(llmID)
	if err != nil {
		return err
	}

	catalogue, err := s.GetCatalogueByID(catalogueID)
	if err != nil {
		return err
	}

	return catalogue.AddLLM(s.DB, llm)
}

func (s *Service) RemoveLLMFromCatalogue(llmID, catalogueID uint) error {
	llm, err := s.GetLLMByID(llmID)
	if err != nil {
		return err
	}

	catalogue, err := s.GetCatalogueByID(catalogueID)
	if err != nil {
		return err
	}

	return catalogue.RemoveLLM(s.DB, llm)
}

func (s *Service) GetCatalogueLLMs(catalogueID uint) (models.LLMs, error) {
	catalogue, err := s.GetCatalogueByID(catalogueID)
	if err != nil {
		return nil, err
	}

	if err := catalogue.GetCatalogueLLMs(s.DB); err != nil {
		return nil, err
	}

	return catalogue.LLMs, nil
}

func (s *Service) GetAllCatalogues(pageSize int, pageNumber int, all bool) (models.Catalogues, int64, int, error) {
	var catalogues models.Catalogues
	totalCount, totalPages, err := catalogues.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return catalogues, totalCount, totalPages, nil
}

func (s *Service) SearchCataloguesByNameStub(stub string) (models.Catalogues, error) {
	var catalogues models.Catalogues
	if err := catalogues.GetByNameStub(s.DB, stub); err != nil {
		return nil, err
	}
	return catalogues, nil
}
