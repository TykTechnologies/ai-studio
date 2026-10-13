package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
)

// requireSecretReadForKeys is the shared write-time check of the reference
// policy (secrets/refpolicy.go): a write that adds the listed references
// (secrets.NewReferences: new, changed or moved $SECRET/ and $ENV/ values)
// needs secrets read, since the value is resolved and sent on for the
// writer. It writes the 403 and returns false when refused.
func requireSecretReadForKeys(c *gin.Context, newRefs []string) bool {
	if len(newRefs) == 0 || authz.Can(c, authz.Read("secrets")) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("secrets")))
	return false
}
