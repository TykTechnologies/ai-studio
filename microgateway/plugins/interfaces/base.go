// plugins/interfaces/base.go
package interfaces

// HookType represents the type of plugin hook
type HookType string

const (
	HookTypePreAuth        HookType = "pre_auth"
	HookTypeAuth           HookType = "auth"
	HookTypePostAuth       HookType = "post_auth"
	HookTypeOnResponse     HookType = "on_response"
	HookTypeDataCollection HookType = "data_collection"
)

// BasePlugin defines the base interface that all plugins must implement
type BasePlugin interface {
	// Initialize initializes the plugin with configuration
	Initialize(config map[string]interface{}) error
	
	// GetHookType returns the hook type this plugin implements
	GetHookType() HookType
	
	// GetName returns the plugin name
	GetName() string
	
	// GetVersion returns the plugin version
	GetVersion() string
	
	// Shutdown performs cleanup when plugin is unloaded
	Shutdown() error
}

// PluginContext provides contextual information for plugin execution
type PluginContext struct {
	RequestID    string                 `json:"request_id"`
	LLMID        uint                   `json:"llm_id"`
	LLMSlug      string                 `json:"llm_slug"`
	Vendor       string                 `json:"vendor"`
	AppID        uint                   `json:"app_id"`
	UserID       uint                   `json:"user_id,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	TraceContext map[string]string      `json:"trace_context,omitempty"`
}

// PluginRequest represents an HTTP request for plugin processing
type PluginRequest struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	RemoteAddr string            `json:"remote_addr"`
	Context    *PluginContext    `json:"context"`
}

// PluginResponse represents a plugin's response/modification to a request
type PluginResponse struct {
	Modified    bool              `json:"modified"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        []byte            `json:"body,omitempty"`
	Block       bool              `json:"block"`        // Stop processing if true
	ErrorMessage string           `json:"error_message,omitempty"`
}