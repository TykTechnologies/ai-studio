package services

import (
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

// validateLLMVendorConfig checks vendor-specific connection settings an LLM
// cannot work without. checkKey is false on an update that keeps the stored
// key, so an LLM saved before a check existed can still be edited.
//
// Vertex (vertexVendor.ValidateConfig, shared with the Microgateway): the
// endpoint is "project:location" or a URL, and the key a Google credential
// JSON or an access token. A key-less Vertex LLM is still saved (Studio
// chat may use the host's credentials) with a warning that gateway routes
// refuse it unless the operator enabled AI_STUDIO_VERTEX_ADC.
func validateLLMVendorConfig(llm *models.LLM, checkKey bool) error {
	if llm.Vendor != models.VERTEX {
		return nil
	}
	warning, err := vertexVendor.ValidateConfig(llm.APIEndpoint, llm.APIKey, true, checkKey)
	if err != nil {
		return &LLMValidationError{Err: err}
	}
	if warning != nil {
		logger.Warnf("Vertex LLM %q: %v", llm.Name, warning)
	}
	return nil
}
