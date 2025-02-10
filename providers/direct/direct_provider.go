package direct

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/providers"
)

// DirectProvider implements OpenAPIProvider for direct spec imports
type DirectProvider struct {
	specs []providers.APISpec
}

// NewDirectProvider creates a new DirectProvider instance
func NewDirectProvider() *DirectProvider {
	return &DirectProvider{
		specs: make([]providers.APISpec, 0),
	}
}

// Name returns the provider's name
func (p *DirectProvider) Name() string {
	return "Direct Import"
}

// Description returns the provider's description
func (p *DirectProvider) Description() string {
	return "Import OpenAPI specifications directly via file upload or URL"
}

// GetAPISpecs retrieves all imported API specifications
func (p *DirectProvider) GetAPISpecs() ([]providers.APISpec, error) {
	return p.specs, nil
}

// ValidateCredentials is a no-op for DirectProvider as it doesn't require credentials
func (p *DirectProvider) ValidateCredentials() error {
	return nil
}

// ImportFromURL imports an OpenAPI spec from a URL
func (p *DirectProvider) ImportFromURL(url string, name string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch spec from URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch spec from URL: status code %d", resp.StatusCode)
	}

	specBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read spec content: %v", err)
	}

	// Validate JSON format
	var jsonContent interface{}
	if err := json.Unmarshal(specBytes, &jsonContent); err != nil {
		return fmt.Errorf("invalid JSON format: %v", err)
	}

	spec := providers.APISpec{
		ID:          fmt.Sprintf("direct-%d", len(p.specs)+1),
		Name:        name,
		Description: fmt.Sprintf("Imported from URL: %s", url),
		Spec:        string(specBytes),
		Source:      url,
	}

	p.specs = append(p.specs, spec)
	return nil
}

// ImportFromFile imports an OpenAPI spec from file content
func (p *DirectProvider) ImportFromFile(content []byte, filename string) error {
	// Validate JSON format
	var jsonContent interface{}
	if err := json.Unmarshal(content, &jsonContent); err != nil {
		return fmt.Errorf("invalid JSON format: %v", err)
	}

	spec := providers.APISpec{
		ID:          fmt.Sprintf("direct-%d", len(p.specs)+1),
		Name:        filename,
		Description: fmt.Sprintf("Imported from file: %s", filename),
		Spec:        string(content),
		Source:      "file",
	}

	p.specs = append(p.specs, spec)
	return nil
}
