package authz

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ShadowMode runs authorization checks alongside the legacy auth system and logs discrepancies.
// It never blocks requests — the legacy system remains the source of truth.
// This is used during migration to validate authorization correctness before switching over.

// ShadowCheck runs an authorization check after the legacy middleware has already allowed the
// request. Logs a warning if the authorizer disagrees with the legacy decision.
// legacyCheck returns the legacy system's decision for the same request.
func ShadowCheck(authz Authorizer, resourceType, relation, resourceID string, userID UserIDFromContext, legacyCheck func(c *gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		uid, ok := userID(c)
		if !ok {
			c.Next()
			return
		}

		allowed, err := authz.CheckByName(c.Request.Context(), uid, relation, resourceType, resourceID)
		if err != nil {
			log.Warn().Err(err).Uint("user_id", uid).
				Str("relation", relation).
				Str("resource", resourceType+":"+resourceID).
				Msg("authz/shadow: check error")
			c.Next()
			return
		}

		legacyAllowed := legacyCheck(c)
		if legacyAllowed != allowed {
			log.Warn().
				Uint("user_id", uid).
				Str("relation", relation).
				Str("resource", resourceType+":"+resourceID).
				Bool("legacy", legacyAllowed).
				Bool("authz", allowed).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY")
		} else {
			log.Debug().
				Uint("user_id", uid).
				Bool("allowed", allowed).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: check consistent")
		}

		c.Next()
	}
}

// ShadowCheckResource runs an authorization check for a resource whose ID comes from a URL
// parameter. Logs a warning on discrepancy with the legacy system.
func ShadowCheckResource(authz Authorizer, resourceType, relation, paramName string, userID UserIDFromContext, legacyCheck func(c *gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		uid, ok := userID(c)
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

		allowed, err := authz.Check(c.Request.Context(), uid, relation, resourceType, uint(id))
		if err != nil {
			log.Warn().Err(err).
				Uint("user_id", uid).
				Str("resource", resourceType+":"+idStr).
				Str("relation", relation).
				Msg("authz/shadow: resource check error")
			c.Next()
			return
		}

		legacyAllowed := legacyCheck(c)
		if legacyAllowed != allowed {
			log.Warn().
				Uint("user_id", uid).
				Str("resource", resourceType+":"+idStr).
				Str("relation", relation).
				Bool("legacy", legacyAllowed).
				Bool("authz", allowed).
				Str("method", c.Request.Method).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY in resource check")
		}

		c.Next()
	}
}

// ShadowCheckOwnership runs an authorization ownership check and compares against the HTTP
// response status from the legacy handler to detect discrepancies.
func ShadowCheckOwnership(authz Authorizer, resourceType, relation, paramName string, userID UserIDFromContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.Enabled() {
			c.Next()
			return
		}

		uid, ok := userID(c)
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

		allowed, err := authz.Check(c.Request.Context(), uid, relation, resourceType, uint(id))
		if err != nil {
			log.Warn().Err(err).
				Uint("user_id", uid).
				Str("resource", resourceType+":"+idStr).
				Msg("authz/shadow: ownership check error")
			c.Next()
			return
		}

		log.Debug().
			Uint("user_id", uid).
			Str("resource", resourceType+":"+idStr).
			Bool("authz_can_use", allowed).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Msg("authz/shadow: ownership pre-check")

		c.Set("authz_shadow_allowed", allowed)
		c.Next()

		legacyDenied := c.Writer.Status() == http.StatusForbidden
		if legacyDenied == allowed {
			log.Warn().
				Uint("user_id", uid).
				Str("resource", resourceType+":"+idStr).
				Bool("authz_allowed", allowed).
				Bool("legacy_denied", legacyDenied).
				Int("status", c.Writer.Status()).
				Str("path", c.Request.URL.Path).
				Msg("authz/shadow: DISCREPANCY in ownership check")
		}
	}
}
