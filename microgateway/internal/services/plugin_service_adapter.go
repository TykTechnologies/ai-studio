// internal/services/plugin_service_adapter.go
package services

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
)

// PluginServiceAdapter adapts PluginService to match the plugins.PluginServiceInterface
// This breaks the circular dependency between services and plugins packages
type PluginServiceAdapter struct {
	pluginService PluginServiceInterface
}

// NewPluginServiceAdapter creates a new adapter for the plugin service
func NewPluginServiceAdapter(pluginService PluginServiceInterface) *PluginServiceAdapter {
	return &PluginServiceAdapter{
		pluginService: pluginService,
	}
}

// GetPlugin implements plugins.PluginServiceInterface
func (a *PluginServiceAdapter) GetPlugin(id uint) (plugins.PluginData, error) {
	dbPlugin, err := a.pluginService.GetPlugin(id)
	if err != nil {
		return plugins.PluginData{}, err
	}

	// Convert database plugin to plugins.PluginData
	configBytes, _ := json.Marshal(dbPlugin.Config)
	
	return plugins.PluginData{
		ID:       dbPlugin.ID,
		Name:     dbPlugin.Name,
		Slug:     dbPlugin.Slug,
		HookType: dbPlugin.HookType,
		Command:  dbPlugin.Command,
		Config:   configBytes,
		Checksum: dbPlugin.Checksum,
		IsActive: dbPlugin.IsActive,
	}, nil
}

// GetPluginsByLLMID implements plugins.PluginServiceInterface
func (a *PluginServiceAdapter) GetPluginsByLLMID(llmID uint) ([]plugins.PluginData, error) {
	// Use GetPluginsForLLM which exists in the interface
	dbPlugins, err := a.pluginService.GetPluginsForLLM(llmID)
	if err != nil {
		return nil, err
	}

	result := make([]plugins.PluginData, len(dbPlugins))
	for i, dbPlugin := range dbPlugins {
		configBytes, _ := json.Marshal(dbPlugin.Config)
		
		result[i] = plugins.PluginData{
			ID:       dbPlugin.ID,
			Name:     dbPlugin.Name,
			Slug:     dbPlugin.Slug,
			HookType: dbPlugin.HookType,
			Command:  dbPlugin.Command,
			Config:   configBytes,
			Checksum: dbPlugin.Checksum,
			IsActive: dbPlugin.IsActive,
		}
	}

	return result, nil
}

// GetPluginsForLLM implements plugins.PluginServiceInterface
func (a *PluginServiceAdapter) GetPluginsForLLM(llmID uint) ([]plugins.PluginData, error) {
	// Use the service interface method directly
	dbPlugins, err := a.pluginService.GetPluginsForLLM(llmID)
	if err != nil {
		return nil, err
	}

	result := make([]plugins.PluginData, len(dbPlugins))
	for i, dbPlugin := range dbPlugins {
		configBytes, _ := json.Marshal(dbPlugin.Config)
		
		result[i] = plugins.PluginData{
			ID:       dbPlugin.ID,
			Name:     dbPlugin.Name,
			Slug:     dbPlugin.Slug,
			HookType: dbPlugin.HookType,
			Command:  dbPlugin.Command,
			Config:   configBytes,
			Checksum: dbPlugin.Checksum,
			IsActive: dbPlugin.IsActive,
		}
	}

	return result, nil
}