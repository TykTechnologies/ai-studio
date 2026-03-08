package authz

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// RequireSystemAdmin returns middleware that checks if the user is a system admin.
// This replaces auth.AdminOnly().
func RequireSystemAdmin(authz Authorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := userFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		allowed, err := authz.CheckByName(c.Request.Context(), user.ID, "admin", "system", "1")
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

// RequireSSOAdmin returns middleware that checks if the user is an SSO admin.
// This replaces auth.SSOOnly().
func RequireSSOAdmin(authz Authorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := userFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		allowed, err := authz.CheckByName(c.Request.Context(), user.ID, "sso_admin", "system", "1")
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

// RequireRelation returns middleware that checks a specific relation on a resource.
// The resource ID is extracted from the URL parameter named paramName.
func RequireRelation(authz Authorizer, resourceType, relation, paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := userFromContext(c)
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

		allowed, err := authz.Check(c.Request.Context(), user.ID, relation, resourceType, uint(id))
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

// RequireCanUse returns middleware that checks "can_use" relation on a resource type.
func RequireCanUse(authz Authorizer, resourceType, paramName string) gin.HandlerFunc {
	return RequireRelation(authz, resourceType, "can_use", paramName)
}

// RequireCanAdmin returns middleware that checks "can_admin" relation on a resource type.
func RequireCanAdmin(authz Authorizer, resourceType, paramName string) gin.HandlerFunc {
	return RequireRelation(authz, resourceType, "can_admin", paramName)
}

func userFromContext(c *gin.Context) (*models.User, bool) {
	u, exists := c.Get("user")
	if !exists {
		return nil, false
	}
	user, ok := u.(*models.User)
	return user, ok
}
