//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// Namespace management endpoints - CE: All return 402 Payment Required

// @Summary List namespaces
// @Description List all namespaces (Enterprise Edition only)
// @Tags namespaces
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/namespaces [get]
// @Security BearerAuth
func (a *API) listNamespaces(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Multi-tenant namespaces require Enterprise Edition"))
}

// @Summary Reload namespace
// @Description Trigger reload for all edges in a namespace (Enterprise Edition only)
// @Tags namespaces
// @Accept json
// @Produce json
// @Param namespace path string true "Namespace name"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/reload [post]
// @Security BearerAuth
func (a *API) triggerNamespaceReload(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Namespace-based reload requires Enterprise Edition"))
}

// @Summary Get namespace edges
// @Description Get all edges in a namespace (Enterprise Edition only)
// @Tags namespaces
// @Accept json
// @Produce json
// @Param namespace path string true "Namespace name"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/edges [get]
// @Security BearerAuth
func (a *API) getNamespaceEdges(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Multi-tenant namespaces require Enterprise Edition"))
}

// @Summary Get reload operation status
// @Description Get status of a specific reload operation
// @Tags namespaces
// @Accept json
// @Produce json
// @Param operation_id path string true "Operation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/reload-operations/{operation_id}/status [get]
// @Security BearerAuth
func (a *API) getReloadOperationStatus(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Namespace reload operations require Enterprise Edition"))
}
