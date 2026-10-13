package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// requireSecretReadForReferences refuses a plugin config write that adds a
// $SECRET/ reference unless the caller may read secrets: AI Studio resolves
// references for the plugin, so writing one hands the plugin (and anything
// that can read the plugin's config) the secret's value. It writes the 403
// and returns false when refused.
func requireSecretReadForReferences(c *gin.Context, old, updated map[string]interface{}) bool {
	if len(services.NewSecretReferences(old, updated)) == 0 {
		return true
	}
	if authz.Can(c, authz.Read("secrets")) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("secrets")))
	return false
}
