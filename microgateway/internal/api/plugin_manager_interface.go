package api

// PluginManagerInterface defines the interface we need from the plugin manager
// This avoids circular imports between api and plugins packages
type PluginManagerInterface interface {
	ExecutePluginChain(llmID uint, hookType string, input interface{}, pluginCtx interface{}) (interface{}, error)
	GetPluginsForLLM(llmID uint, hookType string) (interface{}, error)
	IsPluginLoaded(pluginID uint) bool
	RefreshLLMPluginMapping(llmID uint) error
}
