package tyk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/providers"
)

// TykDashboardProvider implements the OpenAPIProvider interface for Tyk Dashboard
type TykDashboardProvider struct {
	Config providers.ProviderConfig
	client *http.Client
}

// NewTykDashboardProvider creates a new Tyk Dashboard provider instance
func NewTykDashboardProvider(config providers.ProviderConfig) *TykDashboardProvider {
	return &TykDashboardProvider{
		Config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the provider name
func (t *TykDashboardProvider) Name() string {
	return "Tyk Dashboard"
}

// Description returns the provider description
func (t *TykDashboardProvider) Description() string {
	return "Import API specifications from your Tyk Dashboard instance"
}

// ValidateCredentials checks if the provided credentials are valid
func (t *TykDashboardProvider) ValidateCredentials() error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/apis", t.Config.URL), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid credentials: received status code %d", resp.StatusCode)
	}

	return nil
}

// TykAPI represents an API definition from Tyk Dashboard
type TykAPI struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	APIDefinition json.RawMessage `json:"api_definition"`
}

// GetAPISpecs retrieves all API specifications from Tyk Dashboard
func (t *TykDashboardProvider) GetAPISpecs() ([]providers.APISpec, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/apis", t.Config.URL), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error from Tyk Dashboard: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tykAPIs []TykAPI
	if err := json.NewDecoder(resp.Body).Decode(&tykAPIs); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	var specs []providers.APISpec
	for _, api := range tykAPIs {
		// Extract OpenAPI spec from API definition
		var apiDef map[string]interface{}
		if err := json.Unmarshal(api.APIDefinition, &apiDef); err != nil {
			continue
		}

		// Check if API has OpenAPI spec
		openAPISpec, ok := apiDef["openapi_spec"].(string)
		if !ok || openAPISpec == "" {
			continue
		}

		specs = append(specs, providers.APISpec{
			ID:          api.ID,
			Name:        api.Name,
			Description: fmt.Sprintf("API from Tyk Dashboard: %s", api.Name),
			Spec:        openAPISpec,
			Source:      "tyk",
		})
	}

	return specs, nil
}
