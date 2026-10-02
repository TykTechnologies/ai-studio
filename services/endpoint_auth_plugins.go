package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ErrEndpointNotFound is returned when the endpoint an auth plugin list is
// read or set for does not exist.
var ErrEndpointNotFound = errors.New("endpoint not found")

// ErrInvalidEndpointAuthPlugin is returned (wrapped) when a plugin cannot be
// attached to an endpoint as an auth plugin.
var ErrInvalidEndpointAuthPlugin = errors.New("invalid auth plugin")

// endpointNamespace returns the namespace of the endpoint, or
// ErrEndpointNotFound. A plugin endpoint must also serve custom endpoints.
func (s *PluginService) endpointNamespace(objectType string, objectID uint) (string, error) {
	var row struct {
		Namespace string
	}
	var model interface{}
	switch objectType {
	case models.EndpointTypeDatasource:
		model = &models.Datasource{}
	case models.EndpointTypeTool:
		model = &models.Tool{}
	case models.EndpointTypeModelRouter:
		model = &models.ModelRouter{}
	case models.EndpointTypeSemanticRouter:
		model = &models.SemanticRouter{}
	case models.EndpointTypePlugin:
		var plugin models.Plugin
		if err := s.db.First(&plugin, objectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", ErrEndpointNotFound
			}
			return "", err
		}
		if !plugin.SupportsHookType(models.HookTypeCustomEndpoint) {
			return "", fmt.Errorf("%w: plugin %d serves no custom endpoints", ErrEndpointNotFound, objectID)
		}
		return plugin.Namespace, nil
	default:
		return "", fmt.Errorf("%w: unknown endpoint type %q", ErrEndpointNotFound, objectType)
	}
	res := s.db.Model(model).Select("namespace").Where("id = ?", objectID).Limit(1).Scan(&row)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", ErrEndpointNotFound
	}
	return row.Namespace, nil
}

// GetEndpointAuthPlugins returns the auth plugins attached to an endpoint, in
// execution order.
func (s *PluginService) GetEndpointAuthPlugins(objectType string, objectID uint) ([]models.Plugin, error) {
	if _, err := s.endpointNamespace(objectType, objectID); err != nil {
		return nil, err
	}
	ids, err := models.GetEndpointAuthPluginIDs(s.db, objectType, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint auth plugins: %w", err)
	}
	if len(ids) == 0 {
		return []models.Plugin{}, nil
	}
	var found []models.Plugin
	if err := s.db.Where("id IN ?", ids).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("failed to load endpoint auth plugins: %w", err)
	}
	byID := make(map[uint]models.Plugin, len(found))
	for _, p := range found {
		byID[p.ID] = p
	}
	plugins := make([]models.Plugin, 0, len(ids))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			plugins = append(plugins, p)
		}
	}
	return plugins, nil
}

// SetEndpointAuthPlugins replaces an endpoint's auth plugin list; the slice
// order is the execution order. Each plugin must exist, support the auth hook
// and be global or in the endpoint's namespace (an edge only receives those).
// An inactive plugin may be attached: an edge does not load it, so requests
// to the endpoint are refused until it is activated, which is the safe
// direction.
func (s *PluginService) SetEndpointAuthPlugins(objectType string, objectID uint, pluginIDs []uint) error {
	namespace, err := s.endpointNamespace(objectType, objectID)
	if err != nil {
		return err
	}

	seen := make(map[uint]bool, len(pluginIDs))
	for _, id := range pluginIDs {
		if seen[id] {
			return fmt.Errorf("%w: plugin %d is listed twice", ErrInvalidEndpointAuthPlugin, id)
		}
		seen[id] = true

		var plugin models.Plugin
		if err := s.db.First(&plugin, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: plugin %d not found", ErrInvalidEndpointAuthPlugin, id)
			}
			return err
		}
		if !plugin.SupportsHookType(models.HookTypeAuth) {
			return fmt.Errorf("%w: plugin %q does not provide the auth hook", ErrInvalidEndpointAuthPlugin, plugin.Name)
		}
		if plugin.Namespace != "" && plugin.Namespace != namespace {
			return fmt.Errorf("%w: plugin %q is in namespace %q, the endpoint in %q",
				ErrInvalidEndpointAuthPlugin, plugin.Name, plugin.Namespace, namespace)
		}
	}

	if err := models.SetEndpointAuthPlugins(s.db, objectType, objectID, pluginIDs); err != nil {
		return fmt.Errorf("failed to set endpoint auth plugins: %w", err)
	}
	return nil
}
