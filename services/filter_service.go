package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateFilter(name, description string, script []byte, responseFilter bool, namespace string) (*models.Filter, error) {
	filter := &models.Filter{
		Name:           name,
		Description:    description,
		Script:         script,
		ResponseFilter: responseFilter,
		Namespace:      namespace,
	}

	if err := filter.Create(s.DB); err != nil {
		return nil, err
	}

	// Emit event for sync status tracking
	if s.SystemEvents != nil {
		s.SystemEvents.EmitFilterCreated(filter, filter.ID, 0)
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

func (s *Service) UpdateFilter(id uint, name, description string, script []byte, responseFilter bool, namespace string) (*models.Filter, error) {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return nil, err
	}

	filter.Name = name
	filter.Description = description
	filter.Script = script
	filter.ResponseFilter = responseFilter
	filter.Namespace = namespace

	if err := filter.Update(s.DB); err != nil {
		return nil, err
	}

	// Emit event for sync status tracking
	if s.SystemEvents != nil {
		s.SystemEvents.EmitFilterUpdated(filter, filter.ID, 0)
	}

	return filter, nil
}

func (s *Service) DeleteFilter(id uint) error {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return err
	}

	if err := filter.Delete(s.DB); err != nil {
		return err
	}

	// Emit event for sync status tracking
	if s.SystemEvents != nil {
		s.SystemEvents.EmitFilterDeleted(id, 0)
	}

	return nil
}

func (s *Service) GetAllFilters(pageSize int, pageNumber int, all bool) ([]models.Filter, int64, int, error) {
	filter := models.NewFilter()
	return filter.GetAll(s.DB, pageSize, pageNumber, all)
}

// GetAllFiltersWithFilters returns all filters with namespace filtering
// Note: is_active filtering not supported by main Filter model (only microgateway Filter has this field)
func (s *Service) GetAllFiltersWithFilters(pageSize int, pageNumber int, all bool, namespace string) ([]models.Filter, int64, int, error) {
	filter := models.NewFilter()
	return filter.GetAllWithFilters(s.DB, pageSize, pageNumber, all, namespace)
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
