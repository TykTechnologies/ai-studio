// internal/api/handlers/token_handlers.go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// GenerateToken generates a new API token (public endpoint)
func GenerateToken(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.GenerateTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Generate token using token service
		tokenResponse, err := serviceContainer.Token.GenerateToken(&req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "App not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to generate token",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    tokenResponse,
			"message": "Token generated successfully",
		})
	}
}

// ListTokens lists all tokens for an app
func ListTokens(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get app ID from query parameter or auth context
		var appID uint
		if appIDParam := c.Query("app_id"); appIDParam != "" {
			id, err := strconv.ParseUint(appIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid app ID",
					"message": "app_id must be a valid number",
				})
				return
			}
			appID = uint(id)
		} else {
			// If no app_id provided, use the authenticated app's ID
			appID = auth.GetAppID(c)
			if appID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing app ID",
					"message": "app_id query parameter is required",
				})
				return
			}
		}

		tokens, err := serviceContainer.Token.ListTokens(appID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list tokens",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": tokens,
		})
	}
}

// CreateToken creates a new API token (admin endpoint)
func CreateToken(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req services.GenerateTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Generate token using token service
		tokenResponse, err := serviceContainer.Token.GenerateToken(&req)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "App not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create token",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"data":    tokenResponse,
			"message": "Token created successfully",
		})
	}
}

// RevokeToken revokes an API token
func RevokeToken(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing token",
				"message": "Token parameter is required",
			})
			return
		}

		err := serviceContainer.Token.RevokeToken(token)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Token not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to revoke token",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Token revoked successfully",
		})
	}
}

// GetTokenInfo returns information about a token
func GetTokenInfo(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing token",
				"message": "Token parameter is required",
			})
			return
		}

		info, err := serviceContainer.Token.GetTokenInfo(token)
		if err != nil {
			if isNotFoundError(err) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Token not found",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get token info",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": info,
		})
	}
}

// ValidateTokenEndpoint validates a token and returns info (public endpoint)
func ValidateTokenEndpoint(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Token string `json:"token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Validate token
		result, err := serviceContainer.AuthProvider.ValidateToken(req.Token)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to validate token",
				"message": err.Error(),
			})
			return
		}

		if !result.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"valid":   false,
				"error":   "Invalid token",
				"message": result.Error,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"valid":      true,
			"app_id":     result.AppID,
			"scopes":     result.Scopes,
			"expires_at": result.ExpiresAt,
		})
	}
}