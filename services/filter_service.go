package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateFilter(name, description string, script []byte) (*models.Filter, error) {
	filter := &models.Filter{
		Name:        name,
		Description: description,
		Script:      script,
	}

	if err := filter.Create(s.DB); err != nil {
		return nil, err
	}

	return filter, nil
}

func (s *Service) GetFilterByID(id uint) (*models.Filter, error) {
	filter := models.NewFilter()
	if err := filter.Get(s.DB, id); err != nil {
		return nil, err
	}
	return filter, nil
}

func (s *Service) UpdateFilter(id uint, name, description string, script []byte) (*models.Filter, error) {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return nil, err
	}

	filter.Name = name
	filter.Description = description
	filter.Script = script

	if err := filter.Update(s.DB); err != nil {
		return nil, err
	}

	return filter, nil
}

func (s *Service) DeleteFilter(id uint) error {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return err
	}

	return filter.Delete(s.DB)
}

func (s *Service) GetAllFilters(pageSize int, pageNumber int, all bool) ([]models.Filter, int64, int, error) {
	filter := models.NewFilter()
	return filter.GetAll(s.DB, pageSize, pageNumber, all)
}

func (s *Service) GetFilterByName(name string) (*models.Filter, error) {
	filter := models.NewFilter()
	if err := filter.GetByName(s.DB, name); err != nil {
		return nil, err
	}
	return filter, nil
}

func (s *Service) GetFiltersByChatID(chatID uint) ([]*models.Filter, error) {
	chat := &models.Chat{}
	err := chat.Get(s.DB, chatID)
	if err != nil {
		return nil, err
	}

	var filters []*models.Filter
	for i, _ := range chat.Filters {
		filters = append(filters, chat.Filters[i])
	}

	return filters, nil
}
