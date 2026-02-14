package services

import (
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// InjectTestPlugin injects a mock loaded plugin for testing purposes.
// This allows API handler tests to simulate a loaded plugin without
// starting a real plugin process.
func (m *AIStudioPluginManager) InjectTestPlugin(pluginID uint, name string, grpcClient pb.PluginServiceClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loadedPlugins[pluginID] = &LoadedAIStudioPlugin{
		ID:         pluginID,
		Name:       name,
		GRPCClient: grpcClient,
		LoadTime:   time.Now(),
		IsHealthy:  true,
		LastPing:   time.Now(),
	}
}

// RemoveTestPlugin removes a previously injected test plugin.
func (m *AIStudioPluginManager) RemoveTestPlugin(pluginID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.loadedPlugins, pluginID)
}
