package services

import (
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/rs/zerolog/log"
)

// validateVertexLLM applies AI Studio's Vertex rules
// (vertexVendor.ValidateConfig) to the management API. checkEndpoint and
// checkKey say which fields the request sets. A key-less LLM is accepted
// with a warning: this gateway refuses it unless the operator set
// AI_STUDIO_VERTEX_ADC. Errors never quote the key.
func validateVertexLLM(name, endpoint, apiKey string, checkEndpoint, checkKey bool) error {
	warning, err := vertexVendor.ValidateConfig(endpoint, apiKey, checkEndpoint, checkKey)
	if warning != nil {
		log.Warn().Str("llm", name).Msg(warning.Error())
	}
	return err
}
