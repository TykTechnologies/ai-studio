package api

import (
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// requireSecretReadForReferences refuses a plugin config write that adds a
// $SECRET/ reference unless the caller may read secrets: AI Studio resolves
// references for the plugin, so writing one hands the plugin (and anything
// that can read the plugin's config) the secret's value. It writes the 403
// and returns false when refused.
func requireSecretReadForReferences(c *gin.Context, old, updated map[string]interface{}) bool {
	return requireSecretReadForKeys(c, services.NewSecretReferences(old, updated))
}

// requireSecretReadForGuardrailReferences does the same for a guardrail
// filter's connection block: the gateway resolves a $SECRET/ or allowlisted
// $ENV/ reference and sends the value to the endpoint the filter names, so
// adding one (or moving one to another key) needs the right to read secrets.
// old is the stored config, nil on create. A config that is not a guardrail
// config has no connection block and passes; the service validates it.
func requireSecretReadForGuardrailReferences(c *gin.Context, old, updated map[string]interface{}) bool {
	if len(updated) == 0 {
		return true
	}
	newCfg, err := guardrails.ParseConfig(updated)
	if err != nil {
		return true
	}
	var oldConn map[string]string
	if len(old) > 0 {
		if oldCfg, err := guardrails.ParseConfig(old); err == nil {
			oldConn = oldCfg.Connection
		}
	}
	return requireSecretReadForKeys(c, guardrails.NewConnectionReferences(oldConn, newCfg.Connection))
}
