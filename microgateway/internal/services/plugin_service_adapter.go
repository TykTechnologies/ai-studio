// internal/services/plugin_service_adapter.go
package services

import (
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
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

// convertDBPlugin converts a database.Plugin to plugins.PluginData.
func (a *PluginServiceAdapter) convertDBPlugin(dbPlugin database.Plugin) plugins.PluginData {
	configBytes, _ := json.Marshal(dbPlugin.Config)

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

// GetAllActiveGatewayPlugins implements plugins.PluginServiceInterface.
// Returns all active plugins that should run on a gateway: LLM-associated plugins
// plus standalone custom_endpoint plugins, deduplicated by ID.
func (a *PluginServiceAdapter) GetAllActiveGatewayPlugins() ([]plugins.PluginData, error) {
	seen := make(map[uint]bool)
	var result []plugins.PluginData

	// Step 1: get all LLM-associated plugins in a single query
	dbPlugins, err := a.pluginService.GetAllLLMAssociatedPlugins()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM-associated plugins: %w", err)
	}
	for _, dbPlugin := range dbPlugins {
		pd := a.convertDBPlugin(dbPlugin)
		if !pd.HasAnySupportedGatewayHook() {
			continue
		}
		seen[pd.ID] = true
		result = append(result, pd)
	}

	// Step 2: add standalone custom_endpoint plugins
	allPlugins, err := a.GetAllPlugins()
	if err != nil {
		return nil, fmt.Errorf("failed to get all plugins: %w", err)
	}
	for _, p := range allPlugins {
		if seen[p.ID] || !p.IsActive {
			continue
		}
		if p.HookType == "custom_endpoint" || p.SupportsHookType("custom_endpoint") {
			seen[p.ID] = true
			result = append(result, p)
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