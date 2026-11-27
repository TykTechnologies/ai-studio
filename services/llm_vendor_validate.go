package services

import (
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

// validateLLMVendorConfig checks vendor-specific connection settings an LLM
// cannot work without. checkKey is false on an update that keeps the stored
// key, so an LLM saved before a check existed can still be edited.
//
// Vertex: the endpoint is "project:location" or a URL, and the key a Google
// credential JSON or an access token. A key-less Vertex LLM is still saved
// (Studio chat may use the host's credentials) with a warning that gateway
// routes refuse it unless the operator enabled AI_STUDIO_VERTEX_ADC.
func validateLLMVendorConfig(llm *models.LLM, checkKey bool) error {
	if llm.Vendor != models.VERTEX {
		return nil
	}
	if err := vertexVendor.ValidateEndpoint(llm.APIEndpoint); err != nil {
		return &LLMValidationError{Err: err}
	}
	// A secret reference or an encrypted value resolves later.
	if !checkKey || secrets.CheckCredential(llm.APIKey) != nil {
		return nil
	}
	if err := vertexVendor.ValidateKey(llm.APIKey); err != nil {
		if vertexVendor.IsWarning(err) {
			logger.Warnf("Vertex LLM %q: %v", llm.Name, err)
			return nil
		}
		return &LLMValidationError{Err: err}
	}
	return nil
}
