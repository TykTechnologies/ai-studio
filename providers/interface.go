package providers

// SecurityDetails represents authentication details for an API
type SecurityDetails struct {
	Type string `json:"type"`
	Name string `json:"name"`
	In   string `json:"in"`
}

// APISpec represents an OpenAPI specification from any provider
type APISpec struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Spec            string          `json:"spec"`
	Source          string          `json:"source"`
	SecurityDetails SecurityDetails `json:"security_details,omitempty"`
	Operations      []string        `json:"operations,omitempty"`
}

// OpenAPIProvider defines the interface that all API spec providers must implement
type OpenAPIProvider interface {
	// Name returns the provider's name
	Name() string

	// Description returns a human-readable description of the provider
	Description() string

	// GetAPISpecs retrieves all available API specifications
	GetAPISpecs() ([]APISpec, error)

	// ValidateCredentials checks if the provider credentials are valid
	ValidateCredentials() error
}

// ImportMethod represents the method used to import an OpenAPI spec
type ImportMethod struct {
	Type        string `json:"type"`         // "provider", "url", or "file"
	Name        string `json:"name"`         // Display name for the method
	Description string `json:"description"`  // Description of the import method
	Provider    string `json:"provider"`     // Provider ID ("tyk", "direct", etc.)
	NeedsConfig bool   `json:"needs_config"` // Whether this method needs configuration
}

// ProviderConfig represents the configuration for a provider
type ProviderConfig struct {
	URL           string `json:"url"`
	Token         string `json:"token"`
	SelectedAPIID string `json:"selected_api_id,omitempty"`
}

// ImportConfig represents the configuration for direct imports
type ImportConfig struct {
	URL      string `json:"url,omitempty"`       // For URL imports
	Name     string `json:"name"`                // Name for the imported spec
	FileData []byte `json:"file_data,omitempty"` // For file imports
}

// ImportStep represents a step in the import process
type ImportStep struct {
	Type        string         `json:"type"`         // "config", "select_api", "import_method"
	Methods     []ImportMethod `json:"methods"`      // Available import methods for this step
	Provider    string         `json:"provider"`     // Provider ID
	CurrentStep int            `json:"current_step"` // Current step number
	TotalSteps  int            `json:"total_steps"`  // Total number of steps
}
