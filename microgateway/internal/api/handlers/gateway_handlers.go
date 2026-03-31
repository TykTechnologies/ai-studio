// internal/api/handlers/gateway_handlers.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)


// SwaggerHandler serves basic Swagger documentation
// Note: Returns static API documentation - can be enhanced with generated docs later
func SwaggerHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"swagger": "2.0",
			"info": gin.H{
				"title":   "Microgateway API",
				"version": "1.0.0",
				"description": "AI/LLM microgateway management API",
			},
			"host":     c.Request.Host,
			"basePath": "/api/v1",
			"schemes":  []string{"http", "https"},
			"paths": gin.H{
				"/health": gin.H{
					"get": gin.H{
						"summary": "Health check",
						"responses": gin.H{
							"200": gin.H{"description": "Service is healthy"},
						},
					},
				},
				"/api/v1/llms": gin.H{
					"get": gin.H{
						"summary": "List LLMs",
						"responses": gin.H{
							"200": gin.H{"description": "List of LLMs"},
						},
					},
					"post": gin.H{
						"summary": "Create LLM",
						"responses": gin.H{
							"201": gin.H{"description": "LLM created"},
						},
					},
				},
			},
			"message": "Full Swagger documentation will be implemented",
		})
	}
}