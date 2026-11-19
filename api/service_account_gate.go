package api

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// Service accounts (models.AccountTypeService) are automation identities:
// they use the management API (/api/v1) with their API key and nothing
// else. ShowPortal and ShowChat are only UI flags, so the server refuses
// them the Portal and Chat routes itself.

// serviceAccountForbidden answers 403 with code service_account.
func serviceAccountForbidden(c *gin.Context, detail string) {
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Body("Forbidden", detail, services.ServiceAccountCode))
}

// refuseServiceAccount is route middleware (after authentication) that
// refuses a service account outright: the OAuth authorization flow, which
// is a person granting a client access, and the portal analytics group.
func refuseServiceAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, ok := auth.UserFromContext(c); ok && user.IsServiceAccount() {
			serviceAccountForbidden(c, "service accounts cannot use this endpoint")
			return
		}
		c.Next()
	}
}

// serviceAccountCommonAllowed reports whether a /common route (the matched
// pattern) is open to a service account: who it is, its own API key, and
// what it may navigate to.
func serviceAccountCommonAllowed(fullPath string) bool {
	i := strings.Index(fullPath, "/common/")
	if i < 0 {
		return false
	}
	rest := fullPath[i+len("/common"):]
	return rest == "/me" || rest == "/nav" || rest == "/me/api-key" || strings.HasPrefix(rest, "/me/api-key/")
}

// serviceAccountCommonGate is the /common group's middleware (after
// authentication): a service account gets 403 service_account on every
// Portal and Chat route, everything but /common/me, /common/me/api-key/*
// and /common/nav.
func serviceAccountCommonGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, ok := auth.UserFromContext(c); ok && user.IsServiceAccount() &&
			!serviceAccountCommonAllowed(c.FullPath()) {
			serviceAccountForbidden(c, "service accounts have no Portal or Chat access")
			return
		}
		c.Next()
	}
}
