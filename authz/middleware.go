package authz

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UserIDFromContext extracts the authenticated user's ID from the gin context.
// This must be set by the caller to decouple the authz package from application models.
type UserIDFromContext func(c *gin.Context) (uint, bool)

// RequireRelation returns middleware that checks a specific relation on a resource.
// The resource ID is extracted from the URL parameter named paramName.
func RequireRelation(authz Authorizer, resourceType, relation, paramName string, userID UserIDFromContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := userID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		idStr := c.Param(paramName)
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
			return
		}

		allowed, err := authz.Check(c.Request.Context(), uid, relation, resourceType, uint(id))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authorization check failed"})
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		c.Next()
	}
}

// RequireRelationByName returns middleware that checks a relation using a string resource ID.
func RequireRelationByName(authz Authorizer, resourceType, relation, resourceID string, userID UserIDFromContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := userID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		allowed, err := authz.CheckByName(c.Request.Context(), uid, relation, resourceType, resourceID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authorization check failed"})
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		c.Next()
	}
}
