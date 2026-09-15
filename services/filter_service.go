package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// FilterSpec is the full set of attributes a filter is created or updated
// with. Kind selects a script filter (Script) or a guardrail filter (Config,
// validated against the provider catalogue and stored normalised so edges
// receive explicit defaults).
type FilterSpec struct {
	Name           string
	Description    string
	Script         []byte
	ResponseFilter bool
	Namespace      string
	Kind           string
	Config         models.JSONMap
}

// validate checks the spec and returns the kind and config to store.
func (spec FilterSpec) validate() (string, models.JSONMap, error) {
	kind := spec.Kind
	if kind == "" {
		kind = models.FilterKindScript
	}
	switch kind {
	case models.FilterKindScript:
		if len(spec.Script) == 0 {
			return "", nil, fmt.Errorf("script is required for a script filter")
		}
		return kind, nil, nil
	case models.FilterKindGuardrail:
		cfg, err := guardrails.ParseConfig(spec.Config)
		if err != nil {
			return "", nil, err
		}
		cfg, err = guardrails.Normalize(cfg, spec.ResponseFilter)
		if err != nil {
			return "", nil, err
		}
		stored, err := cfg.ToMap()
		if err != nil {
			return "", nil, err
		}
		return kind, stored, nil
	default:
		return "", nil, fmt.Errorf("filter kind %q is not one of script, guardrail", kind)
	}
}

// CreateFilter creates a script filter. Kept for the plugin management API;
// the admin API uses CreateFilterFromSpec.
func (s *Service) CreateFilter(name, description string, script []byte, responseFilter bool, namespace string) (*models.Filter, error) {
	return s.CreateFilterFromSpec(FilterSpec{
		Name:           name,
		Description:    description,
		Script:         script,
		ResponseFilter: responseFilter,
		Namespace:      namespace,
		Kind:           models.FilterKindScript,
	})
}

// CreateFilterFromSpec creates a filter of either kind.
func (s *Service) CreateFilterFromSpec(spec FilterSpec) (*models.Filter, error) {
	kind, config, err := spec.validate()
	if err != nil {
		return nil, err
	}

	filter := &models.Filter{
		Name:           spec.Name,
		Description:    spec.Description,
		Script:         spec.Script,
		ResponseFilter: spec.ResponseFilter,
		Namespace:      spec.Namespace,
		Kind:           kind,
		Config:         config,
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

// UpdateFilter updates the script-filter attributes of a filter. Kept for the
// plugin management API; it leaves Kind and Config untouched so a guardrail
// filter renamed through it keeps its provider configuration.
func (s *Service) UpdateFilter(id uint, name, description string, script []byte, responseFilter bool, namespace string) (*models.Filter, error) {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return nil, err
	}
	return s.UpdateFilterFromSpec(id, FilterSpec{
		Name:           name,
		Description:    description,
		Script:         script,
		ResponseFilter: responseFilter,
		Namespace:      namespace,
		Kind:           filter.Kind,
		Config:         filter.Config,
	})
}

// UpdateFilterFromSpec replaces every attribute of a filter.
func (s *Service) UpdateFilterFromSpec(id uint, spec FilterSpec) (*models.Filter, error) {
	filter, err := s.GetFilterByID(id)
	if err != nil {
		return nil, err
	}

	kind, config, err := spec.validate()
	if err != nil {
		return nil, err
	}

	filter.Name = spec.Name
	filter.Description = spec.Description
	filter.Script = spec.Script
	filter.ResponseFilter = spec.ResponseFilter
	filter.Namespace = spec.Namespace
	filter.Kind = kind
	filter.Config = config

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

// ListFilters is GetAllFilters with the admin list's search and sort. It is
// a separate method because GetAllFilters' signature is fixed by
// ServiceInterface, which the microgateway adapter also implements.
func (s *Service) ListFilters(pageSize int, pageNumber int, all bool, opts ListOptions) ([]models.Filter, int64, int, error) {
	filter := models.NewFilter()
	return filter.GetAll(s.DB, pageSize, pageNumber, all, opts.Scopes("name", "description")...)
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
