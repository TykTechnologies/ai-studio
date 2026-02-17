package plugin_sdk

// ResourceTypeRegistration declares a resource type provided by a plugin.
// Each plugin can register one or more resource types that appear in the
// App creation/editing flow and participate in the governance model.
type ResourceTypeRegistration struct {
	// Slug is the machine-readable identifier (e.g., "mcp_servers").
	// Must be unique per plugin. Used in API paths and DB storage.
	Slug string

	// Name is the human-readable display name (e.g., "MCP Servers").
	Name string

	// Description explains what this resource type is.
	Description string

	// Icon is an optional icon identifier (Material icon name or asset path).
	Icon string

	// HasPrivacyScore indicates whether instances carry a privacy score.
	// If true, instances' privacy scores are subject to the generalized
	// privacy rule: no resource score may exceed the max LLM score in the app.
	HasPrivacyScore bool

	// SupportsSubmissions indicates whether community users can submit
	// new instances of this resource type through the submission workflow.
	SupportsSubmissions bool

	// FormComponent declares how the plugin provides its App Form UI section.
	// If nil, the platform renders a standard multi-select populated via ListResourceInstances.
	FormComponent *ResourceFormComponent
}

// ResourceFormComponent declares a Web Component that the platform will render
// inside the App Create/Edit form for resource selection.
type ResourceFormComponent struct {
	// Tag is the custom element tag name (e.g., "mcp-server-selector").
	Tag string

	// EntryPoint is the path to the JS file relative to plugin assets
	// (e.g., "ui/webc/mcp-selector.js").
	EntryPoint string
}

// ResourceInstance represents a single instance of a plugin resource type.
// Instances are managed by the plugin (stored in plugin KV or external systems)
// and exposed to the platform via the ResourceProvider interface.
type ResourceInstance struct {
	// ID is the plugin-assigned unique identifier for this instance.
	// Stored as a string to accommodate any ID format.
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is an optional description.
	Description string

	// PrivacyScore is the privacy score (0-100). Only meaningful
	// if the ResourceTypeRegistration has HasPrivacyScore=true.
	PrivacyScore int

	// Metadata is arbitrary plugin-defined metadata as JSON bytes.
	// This is opaque to the platform but included in config snapshots.
	Metadata []byte

	// IsActive indicates if this instance is currently usable.
	IsActive bool
}
