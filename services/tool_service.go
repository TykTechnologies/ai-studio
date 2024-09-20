package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateTool creates a new tool with validity checks
func (s *Service) CreateTool(name, description, toolType string, oasSpec []byte, privacyScore int, schemaName, APIKey string) (*models.Tool, error) {
	tool := &models.Tool{
		Name:           name,
		Description:    description,
		ToolType:       toolType,
		OASSpec:        oasSpec,
		PrivacyScore:   privacyScore,
		AuthSchemaName: schemaName,
		AuthKey:        APIKey,
	}

	if err := tool.Create(s.DB); err != nil {
		return nil, err
	}

	return tool, nil
}

// UpdateTool updates an existing tool with validity checks
func (s *Service) UpdateTool(id uint, name, description, toolType string, oasSpec []byte, privacyScore int, schemaName, APIKey string) (*models.Tool, error) {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return nil, err
	}

	tool.Name = name
	tool.Description = description
	tool.ToolType = toolType
	tool.OASSpec = oasSpec
	tool.PrivacyScore = privacyScore
	tool.AuthSchemaName = schemaName
	tool.AuthKey = APIKey

	if err := tool.Update(s.DB); err != nil {
		return nil, err
	}

	return tool, nil
}

// GetToolByID retrieves a tool by its ID
func (s *Service) GetToolByID(id uint) (*models.Tool, error) {
	tool := models.NewTool()
	if err := tool.Get(s.DB, id); err != nil {
		return nil, err
	}
	return tool, nil
}

// DeleteTool deletes a tool
func (s *Service) DeleteTool(id uint) error {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return err
	}

	return tool.Delete(s.DB)
}

// GetToolByName retrieves a tool by its name
func (s *Service) GetToolByName(name string) (*models.Tool, error) {
	tool := models.NewTool()
	if err := tool.GetByName(s.DB, name); err != nil {
		return nil, err
	}
	return tool, nil
}

// GetAllTools retrieves all tools
func (s *Service) GetAllTools() ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetAll(s.DB); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByType retrieves all tools of a specific type
func (s *Service) GetToolsByType(toolType string) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByType(s.DB, toolType); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreMin retrieves all tools with a privacy score greater than or equal to the given minimum
func (s *Service) GetToolsByPrivacyScoreMin(minScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreMin(s.DB, minScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreMax retrieves all tools with a privacy score less than or equal to the given maximum
func (s *Service) GetToolsByPrivacyScoreMax(maxScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreMax(s.DB, maxScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreRange retrieves all tools with a privacy score within the given range
func (s *Service) GetToolsByPrivacyScoreRange(minScore, maxScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreRange(s.DB, minScore, maxScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// SearchTools searches for tools matching the given query in name or description
func (s *Service) SearchTools(query string) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.Search(s.DB, query); err != nil {
		return nil, err
	}
	return tools, nil
}

// AddOperationToTool adds an operation to a tool
func (s *Service) AddOperationToTool(toolID uint, operation string) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	tool.AddOperation(operation)
	return tool.Update(s.DB)
}

// RemoveOperationFromTool removes an operation from a tool
func (s *Service) RemoveOperationFromTool(toolID uint, operation string) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	tool.RemoveOperation(operation)
	return tool.Update(s.DB)
}

// GetToolOperations retrieves all operations associated with a tool
func (s *Service) GetToolOperations(toolID uint) ([]string, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	return tool.GetOperations(), nil
}
