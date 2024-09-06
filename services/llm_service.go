package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateLLM(name, apiKey, apiEndpoint, streamingEndpoint string, privacyScore int, shortDescription, longDescription, externalURL, logoURL string) (*models.LLM, error) {
	llm := &models.LLM{
		Name:              name,
		APIKey:            apiKey,
		APIEndpoint:       apiEndpoint,
		StreamingEndpoint: streamingEndpoint,
		PrivacyScore:      privacyScore,
		ShortDescription:  shortDescription,
		LongDescription:   longDescription,
		ExternalURL:       externalURL,
		LogoURL:           logoURL,
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

func (s *Service) UpdateLLM(id uint, name, apiKey, apiEndpoint, streamingEndpoint string, privacyScore int, shortDescription, longDescription, externalURL, logoURL string) (*models.LLM, error) {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return nil, err
	}

	llm.Name = name
	llm.APIKey = apiKey
	llm.APIEndpoint = apiEndpoint
	llm.StreamingEndpoint = streamingEndpoint
	llm.PrivacyScore = privacyScore
	llm.ShortDescription = shortDescription
	llm.LongDescription = longDescription
	llm.ExternalURL = externalURL
	llm.LogoURL = logoURL

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

func (s *Service) GetAllLLMs() (*models.LLMs, error) {
	llms := &models.LLMs{}
	if err := llms.GetAll(s.DB); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByNameStub(name string) (*models.LLMs, error) {
	llms := &models.LLMs{}
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

func (s *Service) GetLLMsByMaxPrivacyScore(score int) (*models.LLMs, error) {
	llms := &models.LLMs{}
	if err := llms.GetByMaxPrivacyScore(s.DB, score); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByMinPrivacyScore(score int) (*models.LLMs, error) {
	llms := &models.LLMs{}
	if err := llms.GetByMinPrivacyScore(s.DB, score); err != nil {
		return nil, err
	}
	return llms, nil
}

func (s *Service) GetLLMsByPrivacyScoreRange(min, max int) (*models.LLMs, error) {
	llms := &models.LLMs{}
	if err := llms.GetByPrivacyScoreRange(s.DB, min, max); err != nil {
		return nil, err
	}
	return llms, nil
}
