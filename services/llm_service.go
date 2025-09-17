package services

import (
	"fmt"
	"regexp"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func (s *Service) GetLLMByID(id uint) (*models.LLM, error) {
	llm := models.NewLLM()
	if err := llm.Get(s.DB, id); err != nil {
		return nil, err
	}

	llm.APIKey = secrets.GetValue(llm.APIKey, true) // preserve reference for API responses
	llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, true)
	return llm, nil
}

func (s *Service) CreateLLM(name, apiKey, apiEndpoint string, privacyScore int,
	shortDescription, longDescription, logoURL string,
	vendor models.Vendor, active bool, filters []*models.Filter,
	defaultModel string, allowedModels []string, monthlyBudget *float64,
	budgetStartDate *time.Time) (*models.LLM, error) {
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
		AllowedModels:    allowedModels,
		MonthlyBudget:    monthlyBudget,
		BudgetStartDate:  budgetStartDate,
		Namespace:        "", // Default to global namespace
	}

	if err := llm.Create(s.DB); err != nil {
		return nil, err
	}

	return llm, nil
}

// CreateLLMWithNamespace creates a new LLM with namespace support
func (s *Service) CreateLLMWithNamespace(name, apiKey, apiEndpoint string, privacyScore int,
	shortDescription, longDescription, logoURL string,
	vendor models.Vendor, active bool, filters []*models.Filter,
	defaultModel string, allowedModels []string, monthlyBudget *float64,
	budgetStartDate *time.Time, namespace string) (*models.LLM, error) {
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
		AllowedModels:    allowedModels,
		MonthlyBudget:    monthlyBudget,
		BudgetStartDate:  budgetStartDate,
		Namespace:        namespace,
	}

	if err := llm.Create(s.DB); err != nil {
		return nil, err
	}

	return llm, nil
}

func (s *Service) UpdateLLM(id uint, name, apiKey, apiEndpoint string,
	privacyScore int, shortDescription, longDescription, logoURL string,
	vendor models.Vendor, active bool, filters []*models.Filter,
	defaultModel string, allowedModels []string, monthlyBudget *float64,
	budgetStartDate *time.Time) (*models.LLM, error) {
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
	llm.AllowedModels = allowedModels
	llm.MonthlyBudget = monthlyBudget
	llm.BudgetStartDate = budgetStartDate

	if err := llm.Update(s.DB); err != nil {
		return nil, err
	}

	return llm, nil
}

// Add some helper functions for managing allowed models
func (s *Service) AddAllowedModel(id uint, modelPattern string) error {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return err
	}

	// Check if pattern is valid regex
	if _, err := regexp.Compile(modelPattern); err != nil {
		return fmt.Errorf("invalid model pattern: %w", err)
	}

	// Check for duplicates
	for _, existing := range llm.AllowedModels {
		if existing == modelPattern {
			return nil // Pattern already exists
		}
	}

	llm.AllowedModels = append(llm.AllowedModels, modelPattern)
	return llm.Update(s.DB)
}

func (s *Service) RemoveAllowedModel(id uint, modelPattern string) error {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return err
	}

	// Filter out the pattern
	newPatterns := make([]string, 0)
	for _, pattern := range llm.AllowedModels {
		if pattern != modelPattern {
			newPatterns = append(newPatterns, pattern)
		}
	}

	llm.AllowedModels = newPatterns
	return llm.Update(s.DB)
}

func (s *Service) ClearAllowedModels(id uint) error {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return err
	}

	llm.AllowedModels = []string{}
	return llm.Update(s.DB)
}

func (s *Service) GetAllowedModels(id uint) ([]string, error) {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return nil, err
	}

	return llm.AllowedModels, nil
}

// Add a validation helper
func (s *Service) IsModelAllowed(id uint, modelName string) (bool, error) {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return false, err
	}

	if len(llm.AllowedModels) == 0 {
		return true, nil // Empty list means all models are allowed
	}

	for _, pattern := range llm.AllowedModels {
		matched, err := regexp.MatchString(pattern, modelName)
		if err != nil {
			return false, fmt.Errorf("invalid pattern '%s': %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
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

	llm.APIKey = secrets.GetValue(llm.APIKey, false) // false to resolve the actual value
	llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, false)
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

func (s *Service) GetActiveLLMs() ([]models.LLM, error) {
	llms := models.LLMs{}
	if err := llms.GetActiveLLMs(s.DB); err != nil {
		return nil, err
	}

	for i := range llms {
		llms[i].APIKey = secrets.GetValue(llms[i].APIKey, false) // false to resolve the actual value
		llms[i].APIEndpoint = secrets.GetValue(llms[i].APIEndpoint, false)
	}

	return []models.LLM(llms), nil
}

func (s *Service) GetLLMsByNameStub(name string) (models.LLMs, error) {
	llms := models.LLMs{}
	if err := llms.GetByNameStub(s.DB, name); err != nil {
		return nil, err
	}
	return llms, nil
}

// GetLLMsInNamespace returns LLMs in a specific namespace (including global)
func (s *Service) GetLLMsInNamespace(namespace string) ([]models.LLM, error) {
	llms := models.LLMs{}

	query := s.DB.Preload("Filters").Preload("Plugins")
	if namespace == "" {
		// Global namespace - only global LLMs
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := query.Find(&llms).Error; err != nil {
		return nil, err
	}

	for i := range llms {
		llms[i].APIKey = secrets.GetValue(llms[i].APIKey, false)
		llms[i].APIEndpoint = secrets.GetValue(llms[i].APIEndpoint, false)
	}

	return []models.LLM(llms), nil
}

// GetActiveLLMsInNamespace returns active LLMs in a specific namespace (including global)
func (s *Service) GetActiveLLMsInNamespace(namespace string) ([]models.LLM, error) {
	llms := models.LLMs{}

	query := s.DB.Preload("Filters").Preload("Plugins").Where("active = ?", true)
	if namespace == "" {
		// Global namespace - only global LLMs
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := query.Find(&llms).Error; err != nil {
		return nil, err
	}

	for i := range llms {
		llms[i].APIKey = secrets.GetValue(llms[i].APIKey, false)
		llms[i].APIEndpoint = secrets.GetValue(llms[i].APIEndpoint, false)
	}

	return []models.LLM(llms), nil
}

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
