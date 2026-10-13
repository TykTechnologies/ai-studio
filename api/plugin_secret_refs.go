package api

import (
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
