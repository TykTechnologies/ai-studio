package grpc

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// llmMetadataForEdge is an LLM's metadata as an edge receives it (aa5):
// string values with references resolved (secrets.ForEdgeCredential), and
// the value of each credential key (models.IsCredentialMetadataKey, such as
// Bedrock's AWS keys) encrypted with the edge encryption key into the
// returned map instead of the metadata JSON. In the JSON such a key holds
// secrets.RefusedCredential, so an edge that reads only the JSON (older
// than 2.3) fails closed instead of falling back to its own credentials.
//
// The encryption is deterministic (encryptForMicrogateway), so an unchanged
// LLM gives an unchanged snapshot checksum. An empty value is not a
// credential and stays in the JSON. An error means a value could not be
// encrypted; the caller leaves the LLM out rather than send it in clear.
func (s *ControlServer) llmMetadataForEdge(metadata models.JSONMap) (string, map[string]string, error) {
	if metadata == nil {
		return "", nil, nil
	}
	resolved := make(models.JSONMap, len(metadata))
	var encrypted map[string]string
	for k, v := range metadata {
		strVal, ok := v.(string)
		if !ok {
			resolved[k] = v
			continue
		}
		// Metadata carries credentials (Bedrock keys), which vendor clients
		// replace with their own when empty.
		value := secrets.ForEdgeCredential(secrets.ResolveRef(strVal))
		if value == "" || !models.IsCredentialMetadataKey(k) {
			resolved[k] = value
			continue
		}
		ciphertext, err := s.encryptForMicrogateway(value)
		if err != nil {
			return "", nil, err
		}
		if encrypted == nil {
			encrypted = make(map[string]string)
		}
		encrypted[k] = ciphertext
		resolved[k] = secrets.RefusedCredential
	}
	metadataBytes, err := json.Marshal(resolved)
	if err != nil {
		return "", nil, err
	}
	return string(metadataBytes), encrypted, nil
}
