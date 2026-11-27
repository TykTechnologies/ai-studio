package services

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/rs/zerolog/log"
)

// validateVertexLLM checks a Vertex LLM's endpoint ("project:location" or a
// URL) and, when one is given, its key (a Google credential JSON or an
// access token), with the same rules as AI Studio. A key-less LLM is
// accepted with a warning: this gateway refuses it unless the operator set
// AI_STUDIO_VERTEX_ADC. checkEndpoint is false on an update that leaves the
// endpoint unchanged. Errors never quote the key.
func validateVertexLLM(name, endpoint, apiKey string, checkEndpoint, keyGiven bool) error {
	if checkEndpoint {
		if err := vertexVendor.ValidateEndpoint(endpoint); err != nil {
			return err
		}
	}
	if !keyGiven || secrets.CheckCredential(apiKey) != nil {
		return nil
	}
	if err := vertexVendor.ValidateKey(apiKey); err != nil {
		if vertexVendor.IsWarning(err) {
			log.Warn().Str("llm", name).Msg(err.Error())
			return nil
		}
		return err
	}
	return nil
}
