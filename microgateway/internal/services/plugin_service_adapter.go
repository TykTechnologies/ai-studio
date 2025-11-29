// internal/services/plugin_service_adapter.go
package services

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
)

// PluginServiceAdapter adapts PluginService to match the plugins.PluginServiceInterface
// This breaks the circular dependency between services and plugins packages
type PluginServiceAdapter struct {
	pluginService     PluginServiceInterface
	managementService ManagementServiceInterface
}

// NewPluginServiceAdapter creates a new adapter for the plugin service
func NewPluginServiceAdapter(pluginService PluginServiceInterface) *PluginServiceAdapter {
	return &PluginServiceAdapter{
		pluginService: pluginService,
	}
}

// SetManagementService sets the management service for LLM queries
func (a *PluginServiceAdapter) SetManagementService(mgmt ManagementServiceInterface) {
	a.managementService = mgmt
}

// GetPlugin implements plugins.PluginServiceInterface
func (a *PluginServiceAdapter) GetPlugin(id uint) (plugins.PluginData, error) {
	dbPlugin, err := a.pluginService.GetPlugin(id)
	if err != nil {
		return plugins.PluginData{}, err
	}

	// Convert database plugin to plugins.PluginData
	configBytes, _ := json.Marshal(dbPlugin.Config)

	// Parse hook_types from JSON
	var hookTypes []string
	if len(dbPlugin.HookTypes) > 0 {
		_ = json.Unmarshal(dbPlugin.HookTypes, &hookTypes)
	}

	return plugins.PluginData{
		ID:        dbPlugin.ID,
		Name:      dbPlugin.Name,
		HookType:  dbPlugin.HookType,
		HookTypes: hookTypes,
		Command:   dbPlugin.Command,
		Config:    configBytes,
		Checksum:  dbPlugin.Checksum,
		IsActive:  dbPlugin.IsActive,
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

		// Parse hook_types from JSON
		var hookTypes []string
		if len(dbPlugin.HookTypes) > 0 {
			_ = json.Unmarshal(dbPlugin.HookTypes, &hookTypes)
		}

		result[i] = plugins.PluginData{
			ID:        dbPlugin.ID,
			Name:      dbPlugin.Name,
			HookType:  dbPlugin.HookType,
			HookTypes: hookTypes,
			Command:   dbPlugin.Command,
			Config:    configBytes,
			Checksum:  dbPlugin.Checksum,
			IsActive:  dbPlugin.IsActive,
		}
	}

	return result, nil
}

// GetAllPlugins implements plugins.PluginServiceInterface
func (a *PluginServiceAdapter) GetAllPlugins() ([]plugins.PluginData, error) {
	// Get all active plugins from the database (page=0, limit=1000 to get all)
	dbPlugins, _, err := a.pluginService.ListPlugins(0, 1000, "", true) // hookType="", active=true
	if err != nil {
		return nil, err
	}

	result := make([]plugins.PluginData, len(dbPlugins))
	for i, dbPlugin := range dbPlugins {
		configBytes, _ := json.Marshal(dbPlugin.Config)

		// Parse hook_types from JSON
		var hookTypes []string
		if len(dbPlugin.HookTypes) > 0 {
			_ = json.Unmarshal(dbPlugin.HookTypes, &hookTypes)
		}

		result[i] = plugins.PluginData{
			ID:        dbPlugin.ID,
			Name:      dbPlugin.Name,
			HookType:  dbPlugin.HookType,
			HookTypes: hookTypes,
			Command:   dbPlugin.Command,
			Config:    configBytes,
			Checksum:  dbPlugin.Checksum,
			IsActive:  dbPlugin.IsActive,
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

		// Parse hook_types from JSON
		var hookTypes []string
		if len(dbPlugin.HookTypes) > 0 {
			_ = json.Unmarshal(dbPlugin.HookTypes, &hookTypes)
		}

		result[i] = plugins.PluginData{
			ID:        dbPlugin.ID,
			Name:      dbPlugin.Name,
			HookType:  dbPlugin.HookType,
			HookTypes: hookTypes,
			Command:   dbPlugin.Command,
			Config:    configBytes,
			Checksum:  dbPlugin.Checksum,
			IsActive:  dbPlugin.IsActive,
		}
	}

	return result, nil
}

// GetAllLLMIDs implements plugins.PluginServiceInterface
// Returns all LLM IDs for pre-warming plugins assigned to LLMs
func (a *PluginServiceAdapter) GetAllLLMIDs() ([]uint, error) {
	if a.managementService == nil {
		return nil, nil // No management service, return empty list
	}

	// Get all active LLMs (page 0 = all, limit 10000 = get all)
	llms, _, err := a.managementService.ListLLMs(0, 10000, "", true)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, len(llms))
	for i, llm := range llms {
		ids[i] = llm.ID
	}

	return ids, nil
}