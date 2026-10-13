package api

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/gin-gonic/gin"
)

// Credential fields that may hold $SECRET/ or $ENV/ references. The value of
// such a reference is resolved and sent to an endpoint the writer chooses
// (the LLM's endpoint, the tool's server, the datasource's host), so adding
// or changing one needs secrets read (requireSecretReadForKeys). A field
// that keeps its stored reference, or is sent back as the redacted
// placeholder, adds nothing.

func llmCredentialFields(apiKey, endpoint string, metadata map[string]interface{}) map[string]string {
	fields := map[string]string{"api_key": apiKey, "api_endpoint": endpoint}
	for k, v := range metadata {
		if s, ok := v.(string); ok {
			fields["metadata."+k] = s
		}
	}
	return fields
}

func datasourceCredentialFields(connString, connAPIKey, embedURL, embedAPIKey string) map[string]string {
	return map[string]string{
		"db_conn_string": connString, "db_conn_api_key": connAPIKey,
		"embed_url": embedURL, "embed_api_key": embedAPIKey,
	}
}

func embedderCredentialFields(e *models.Embedder) map[string]string {
	if e == nil {
		return nil
	}
	return map[string]string{"api_key": e.APIKey, "endpoint": e.Endpoint}
}

// requireSecretReadForCredentials refuses (403) a write that adds or changes
// a reference in updated compared to old (nil for a new object).
func requireSecretReadForCredentials(c *gin.Context, old, updated map[string]string) bool {
	return requireSecretReadForKeys(c, secrets.NewReferences(old, updated))
}
