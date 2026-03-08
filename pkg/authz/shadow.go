package authz

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ShadowMode runs OpenFGA checks alongside the legacy auth system and logs discrepancies.
// It never blocks requests — the legacy system remains the source of truth.
// This is used in Phase 2 to validate OpenFGA correctness before switching over.

// ShadowCheckAdmin runs an OpenFGA admin check after the legacy AdminOnly middleware
// has already allowed the request. Logs a warning if OpenFGA would have denied it.
func ShadowCheckAdmin(authz Authorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		user, ok := userFromContext(c)
		if !ok {
			c.Next()
			return
		}

		allowed, err := authz.CheckStr(c.Request.Context(), user.ID, "admin", "system", "1")
		if err != nil {
			log.Warn().Err(err).Uint("user_id", user.ID).
				Msg("authz/shadow: admin check error")
			c.Next()
			return
		}

		legacyAllowed := user.IsAdmin
		if legacyAllowed != allowed {
			log.Warn().
				Uint("user_id", user.ID).
				Bool("legacy", legacyAllowed).
				Bool("openfga", allowed).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY in admin check")
		} else {
			log.Debug().
				Uint("user_id", user.ID).
				Bool("allowed", allowed).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: admin check consistent")
		}

		c.Next()
	}
}

// ShadowCheckResource runs an OpenFGA check for a specific resource after the request
// has been allowed by legacy auth. Logs a warning on discrepancy.
func ShadowCheckResource(authz Authorizer, objectType, relation, paramName string, legacyCheck func(c *gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		user, ok := userFromContext(c)
		if !ok {
			c.Next()
			return
		}

		idStr := c.Param(paramName)
		if idStr == "" {
			c.Next()
			return
		}

		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.Next()
			return
		}

		allowed, err := authz.Check(c.Request.Context(), user.ID, relation, objectType, uint(id))
		if err != nil {
			log.Warn().Err(err).
				Uint("user_id", user.ID).
				Str("object", objectType+":"+idStr).
				Str("relation", relation).
				Msg("authz/shadow: resource check error")
			c.Next()
			return
		}

		legacyAllowed := legacyCheck(c)
		if legacyAllowed != allowed {
			log.Warn().
				Uint("user_id", user.ID).
				Str("object", objectType+":"+idStr).
				Str("relation", relation).
				Bool("legacy", legacyAllowed).
				Bool("openfga", allowed).
				Str("method", c.Request.Method).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY in resource check")
		}

		c.Next()
	}
}

// ShadowCheckOwnership runs an OpenFGA ownership check alongside the legacy inline
// check (resource.UserID == currentUser.ID). Logs discrepancies.
func ShadowCheckOwnership(authz Authorizer, objectType, paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		user, ok := userFromContext(c)
		if !ok {
			c.Next()
			return
		}

		idStr := c.Param(paramName)
		if idStr == "" {
			c.Next()
			return
		}

		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.Next()
			return
		}

		// Check both can_use (includes owner) and editor (includes owner).
		allowed, err := authz.Check(c.Request.Context(), user.ID, "can_use", objectType, uint(id))
		if err != nil {
			log.Warn().Err(err).
				Uint("user_id", user.ID).
				Str("object", objectType+":"+idStr).
				Msg("authz/shadow: ownership check error")
			c.Next()
			return
		}

		// Log the OpenFGA result. We can't know the legacy result here (it happens
		// later in the handler), so we log the OpenFGA decision for post-hoc comparison.
		log.Debug().
			Uint("user_id", user.ID).
			Str("object", objectType+":"+idStr).
			Bool("openfga_can_use", allowed).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Msg("authz/shadow: ownership pre-check")

		// Store the result in context for the handler to compare if desired.
		c.Set("authz_shadow_allowed", allowed)
		c.Next()

		// After handler runs, check if the response was 403 (legacy denied).
		legacyDenied := c.Writer.Status() == http.StatusForbidden
		if legacyDenied == allowed {
			// Legacy denied but OpenFGA allowed, or vice versa.
			log.Warn().
				Uint("user_id", user.ID).
				Str("object", objectType+":"+idStr).
				Bool("openfga_allowed", allowed).
				Bool("legacy_denied", legacyDenied).
				Int("status", c.Writer.Status()).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY in ownership check")
		}
	}
}
