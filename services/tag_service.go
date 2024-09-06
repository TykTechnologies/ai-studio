package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateTag(name string) (*models.Tag, error) {
	tag := &models.Tag{
		Name: name,
	}

	if err := tag.Create(s.DB); err != nil {
		return nil, err
	}

	return tag, nil
}

func (s *Service) GetTagByID(id uint) (*models.Tag, error) {
	tag := models.NewTag()
	if err := tag.Get(s.DB, id); err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Service) UpdateTag(id uint, name string) (*models.Tag, error) {
	tag, err := s.GetTagByID(id)
	if err != nil {
		return nil, err
	}

	tag.Name = name

	if err := tag.Update(s.DB); err != nil {
		return nil, err
	}

	return tag, nil
}

func (s *Service) DeleteTag(id uint) error {
	tag, err := s.GetTagByID(id)
	if err != nil {
		return err
	}

	return tag.Delete(s.DB)
}

func (s *Service) GetAllTags() (models.Tags, error) {
	var tags models.Tags
	if err := tags.GetAll(s.DB); err != nil {
		return nil, err
	}
	return tags, nil
}

func (s *Service) SearchTagsByNameStub(stub string) (models.Tags, error) {
	var tags models.Tags
	if err := tags.GetByNameStub(s.DB, stub); err != nil {
		return nil, err
	}
	return tags, nil
}

func (s *Service) GetTagByName(name string) (*models.Tag, error) {
	tag := models.NewTag()
	if err := tag.GetByName(s.DB, name); err != nil {
		return nil, err
	}
	return tag, nil
}
