package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateLLM(name, apiKey, apiEndpoint string, privacyScore int,
	shortDescription, longDescription, logoURL string,
	vendor models.Vendor, active bool, filters []*models.Filter, defaultModel string) (*models.LLM, error) {
	llm := &models.LLM{
		Name:             name,
		APIKey:           apiKey,
		APIEndpoint:      apiEndpoint,
		PrivacyScore:     privacyScore,
		ShortDescription: shortDescription,
		LongDescription:  longDescription,
		LogoURL:          logoURL,
		Vendor:           vendor,
		Active:           active,
		Filters:          filters,
		DefaultModel:     defaultModel,
	}

	if err := llm.Create(s.DB); err != nil {
		return nil, err
	}

	return llm, nil
}

func (s *Service) GetLLMByID(id uint) (*models.LLM, error) {
	llm := models.NewLLM()
	if err := llm.Get(s.DB, id); err != nil {
		return nil, err
	}
	return llm, nil
}

func (s *Service) UpdateLLM(id uint, name, apiKey, apiEndpoint string,
	privacyScore int, shortDescription, longDescription, logoURL string,
	vendor models.Vendor, active bool, filters []*models.Filter, defaultModel string) (*models.LLM, error) {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return nil, err
	}

	llm.Name = name
	llm.APIKey = apiKey
	llm.APIEndpoint = apiEndpoint
	llm.PrivacyScore = privacyScore
	llm.ShortDescription = shortDescription
	llm.LongDescription = longDescription
	llm.LogoURL = logoURL
	llm.Vendor = vendor
	llm.Active = active
	llm.Filters = filters
	llm.DefaultModel = defaultModel

	if err := llm.Update(s.DB); err != nil {
		return nil, err
	}

	return llm, nil
}

// The following functions remain unchanged
func (s *Service) DeleteLLM(id uint) error {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return err
	}

	return llm.Delete(s.DB)
}

func (s *Service) GetLLMByName(name string) (*models.LLM, error) {
	llm := models.NewLLM()
	if err := llm.GetByName(s.DB, name); err != nil {
		return nil, err
	}
	return llm, nil
}

func (s *Service) GetAllLLMs(pageSize int, pageNumber int, all bool) (models.LLMs, int64, int, error) {
	var llms models.LLMs
	totalCount, totalPages, err := llms.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return llms, totalCount, totalPages, nil
}

func (s *Service) GetActiveLLMs() (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetActiveLLMs(s.DB); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByNameStub(name string) (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetByNameStub(s.DB, name); err != nil {
		return nil, err
	}
	return llms, nil
}

// Remove this function as it's a duplicate of GetAllLLMs
// func (s *Service) GetAllLLms() (*models.LLMs, error) {
// 	llms := &models.LLMs{}
// 	if err := llms.GetAll(s.DB); err != nil {
// 		return nil, err
// 	}
// 	return llms, nil
// }

func (s *Service) GetLLMsByMaxPrivacyScore(score int) (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetByMaxPrivacyScore(s.DB, score); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByMinPrivacyScore(score int) (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetByMinPrivacyScore(s.DB, score); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByPrivacyScoreRange(min, max int) (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetByPrivacyScoreRange(s.DB, min, max); err != nil {
		return nil, err
	}
	return llms, nil
}
