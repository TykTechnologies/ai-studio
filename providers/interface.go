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

// ProviderConfig represents the configuration for a provider
type ProviderConfig struct {
	URL           string `json:"url"`
	Token         string `json:"token"`
	SelectedAPIID string `json:"selected_api_id,omitempty"`
}
