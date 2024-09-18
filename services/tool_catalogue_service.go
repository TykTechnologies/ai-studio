package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateToolCatalogue(name, shortDescription, longDescription, icon string) (*models.ToolCatalogue, error) {
	toolCatalogue := &models.ToolCatalogue{
		Name:             name,
		ShortDescription: shortDescription,
		LongDescription:  longDescription,
		Icon:             icon,
	}

	if err := toolCatalogue.Create(s.DB); err != nil {
		return nil, err
	}

	return toolCatalogue, nil
}

func (s *Service) GetToolCatalogueByID(id uint) (*models.ToolCatalogue, error) {
	toolCatalogue := models.NewToolCatalogue()
	if err := toolCatalogue.Get(s.DB, id); err != nil {
		return nil, err
	}
	return toolCatalogue, nil
}

func (s *Service) UpdateToolCatalogue(id uint, name, shortDescription, longDescription, icon string) (*models.ToolCatalogue, error) {
	toolCatalogue, err := s.GetToolCatalogueByID(id)
	if err != nil {
		return nil, err
	}

	toolCatalogue.Name = name
	toolCatalogue.ShortDescription = shortDescription
	toolCatalogue.LongDescription = longDescription
	toolCatalogue.Icon = icon

	if err := toolCatalogue.Update(s.DB); err != nil {
		return nil, err
	}

	return toolCatalogue, nil
}

func (s *Service) DeleteToolCatalogue(id uint) error {
	toolCatalogue, err := s.GetToolCatalogueByID(id)
	if err != nil {
		return err
	}

	return toolCatalogue.Delete(s.DB)
}

func (s *Service) GetAllToolCatalogues() (models.ToolCatalogues, error) {
	var toolCatalogues models.ToolCatalogues
	if err := toolCatalogues.GetAll(s.DB); err != nil {
		return nil, err
	}
	return toolCatalogues, nil
}

func (s *Service) SearchToolCatalogues(query string) (models.ToolCatalogues, error) {
	var toolCatalogues models.ToolCatalogues
	if err := toolCatalogues.Search(s.DB, query); err != nil {
		return nil, err
	}
	return toolCatalogues, nil
}

func (s *Service) GetToolCataloguesByTag(tagName string) (models.ToolCatalogues, error) {
	var toolCatalogues models.ToolCatalogues
	if err := toolCatalogues.GetByTag(s.DB, tagName); err != nil {
		return nil, err
	}
	return toolCatalogues, nil
}

func (s *Service) AddToolToToolCatalogue(toolID, toolCatalogueID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	return toolCatalogue.AddTool(s.DB, tool)
}

func (s *Service) RemoveToolFromToolCatalogue(toolID, toolCatalogueID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	return toolCatalogue.RemoveTool(s.DB, tool)
}

func (s *Service) GetToolCatalogueTools(toolCatalogueID uint) (models.Tools, error) {
	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return nil, err
	}

	return toolCatalogue.Tools, nil
}

func (s *Service) AddTagToToolCatalogue(tagID, toolCatalogueID uint) error {
	tag, err := s.GetTagByID(tagID)
	if err != nil {
		return err
	}

	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	return toolCatalogue.AddTag(s.DB, tag)
}

func (s *Service) RemoveTagFromToolCatalogue(tagID, toolCatalogueID uint) error {
	tag, err := s.GetTagByID(tagID)
	if err != nil {
		return err
	}

	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	return toolCatalogue.RemoveTag(s.DB, tag)
}

func (s *Service) GetToolCatalogueTags(toolCatalogueID uint) (models.Tags, error) {
	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return nil, err
	}

	return toolCatalogue.Tags, nil
}
