//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// EdgeResponse represents an edge instance in API responses
// CE: Namespace field is omitted
type EdgeResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		EdgeID        string                 `json:"edge_id"`
		// Namespace field REMOVED in CE
		Version       string                 `json:"version"`
		BuildHash     string                 `json:"build_hash"`
		Metadata      map[string]interface{} `json:"metadata"`
		LastHeartbeat *time.Time             `json:"last_heartbeat"`
		Status        string                 `json:"status"`
		SessionID     string                 `json:"session_id"`
		CreatedAt     time.Time              `json:"created_at"`
		UpdatedAt     time.Time              `json:"updated_at"`
	} `json:"attributes"`
}

// EdgeListResponse represents a list of edges
type EdgeListResponse struct {
	Data []EdgeResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// @Summary List edge instances
// @Description Get a list of registered edge instances (Community Edition - all in "default" namespace)
// @Tags edges
// @Accept json
// @Produce json
// @Param status query string false "Filter by status (registered, connected, disconnected, unhealthy)"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} EdgeListResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges [get]
// @Security BearerAuth
func (a *API) listEdges(c *gin.Context) {
	// CE: Ignore namespace query param (all edges are in "default")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// List edges (all in "default" namespace)
	edges, totalCount, err := a.service.EdgeService.ListEdges("", status, page, limit)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	// Calculate pagination
	totalPages := int(totalCount) / limit
	if int(totalCount)%limit != 0 {
		totalPages++
	}

	// Serialize response WITHOUT namespace field
	response := EdgeListResponse{
		Data: make([]EdgeResponse, len(edges)),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: totalPages,
			PageSize:   limit,
			PageNumber: page,
		},
	}

	for i, edge := range edges {
		response.Data[i] = serializeEdgeWithHealth(&edge)
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get edge instance by ID
// @Description Get details of a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 200 {object} EdgeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/{edge_id} [get]
// @Security BearerAuth
func (a *API) getEdge(c *gin.Context) {
	edgeID := c.Param("edge_id")

	// Security: Validate edge_id parameter
	if err := validateEdgeID(edgeID); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	edge, err := a.service.EdgeService.GetEdgeByEdgeID(edgeID)
	if err != nil {
		if err.Error() == "failed to get edge: record not found" {
			helpers.SendErrorResponse(c, helpers.NewNotFoundError("Edge instance not found"))
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	// Serialize WITHOUT namespace field
	response := serializeEdge(edge)
	c.JSON(http.StatusOK, response)
}

// @Summary Reload edge configuration
// @Description Trigger a configuration reload for a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/{edge_id}/reload [post]
// @Security BearerAuth
func (a *API) triggerEdgeReload(c *gin.Context) {
	edgeID := c.Param("edge_id")

	// Security: Validate edge_id parameter
	if err := validateEdgeID(edgeID); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	// Trigger reload via control server
	err := a.controlServer.TriggerReload([]string{edgeID})
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to trigger reload: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Configuration reload triggered",
		"edge_id": edgeID,
	})
}

// @Summary Delete edge instance
// @Description Delete a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/{edge_id} [delete]
// @Security BearerAuth
func (a *API) deleteEdge(c *gin.Context) {
	edgeID := c.Param("edge_id")

	// Security: Validate edge_id parameter
	if err := validateEdgeID(edgeID); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	err := a.service.EdgeService.DeleteEdge(edgeID)
	if err != nil {
		if err.Error() == "edge not found" {
			helpers.SendErrorResponse(c, helpers.NewNotFoundError("Edge instance not found"))
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Edge instance deleted successfully",
	})
}

// @Summary List reload operations
// @Description Get a list of reload operations and their status
// @Tags edges
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/reload-operations [get]
// @Security BearerAuth
func (a *API) listReloadOperations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	operations, totalCount, err := a.service.EdgeService.ListReloadOperations(page, limit)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": operations,
		"meta": gin.H{
			"total_count": totalCount,
			"total_pages": (totalCount + int64(limit) - 1) / int64(limit),
			"page_size":   limit,
			"page_number": page,
		},
	})
}

// @Summary Reload all edge gateways
// @Description Trigger a configuration reload for all edge gateways (CE: works for single "default" namespace)
// @Tags edges
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/reload-all [post]
// @Security BearerAuth
func (a *API) reloadAllEdges(c *gin.Context) {
	// CE: This works - reloads all edges (all in "default" namespace anyway)
	err := a.controlServer.TriggerGlobalReload()
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to trigger global reload: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Global reload triggered for all edge gateways",
	})
}

// Namespace management endpoints - CE: Returns 402 Payment Required

// @Summary List namespaces
// @Description List all namespaces (Enterprise Edition only)
// @Tags edges
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
// @Tags edges
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
// @Tags edges
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
// @Tags edges
// @Accept json
// @Produce json
// @Param operation_id path string true "Operation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/reload-operations/{operation_id}/status [get]
// @Security BearerAuth
func (a *API) getReloadOperationStatus(c *gin.Context) {
	operationID := c.Param("operation_id")

	status, err := a.service.EdgeService.GetReloadOperationStatus(operationID)
	if err != nil {
		if err.Error() == "operation not found" {
			helpers.SendErrorResponse(c, helpers.NewNotFoundError("Reload operation not found"))
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": status,
	})
}

// Helper functions

// serializeEdge converts an EdgeInstance model to API response format (CE: without namespace)
func serializeEdge(edge *models.EdgeInstance) EdgeResponse {
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.ID), 10),
		Attributes: struct {
			EdgeID        string                 `json:"edge_id"`
			Version       string                 `json:"version"`
			BuildHash     string                 `json:"build_hash"`
			Metadata      map[string]interface{} `json:"metadata"`
			LastHeartbeat *time.Time             `json:"last_heartbeat"`
			Status        string                 `json:"status"`
			SessionID     string                 `json:"session_id"`
			CreatedAt     time.Time              `json:"created_at"`
			UpdatedAt     time.Time              `json:"updated_at"`
		}{
			EdgeID:        edge.EdgeID,
			Version:       edge.Version,
			BuildHash:     edge.BuildHash,
			Metadata:      edge.Metadata,
			LastHeartbeat: edge.LastHeartbeat,
			Status:        edge.Status,
			SessionID:     edge.SessionID,
			CreatedAt:     edge.CreatedAt,
			UpdatedAt:     edge.UpdatedAt,
		},
	}
}

// serializeEdgeWithHealth converts an EdgeInstanceWithHealth to API response format (CE: without namespace)
func serializeEdgeWithHealth(edge *services.EdgeInstanceWithHealth) EdgeResponse {
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.EdgeInstance.ID), 10),
		Attributes: struct {
			EdgeID        string                 `json:"edge_id"`
			Version       string                 `json:"version"`
			BuildHash     string                 `json:"build_hash"`
			Metadata      map[string]interface{} `json:"metadata"`
			LastHeartbeat *time.Time             `json:"last_heartbeat"`
			Status        string                 `json:"status"`
			SessionID     string                 `json:"session_id"`
			CreatedAt     time.Time              `json:"created_at"`
			UpdatedAt     time.Time              `json:"updated_at"`
		}{
			EdgeID:        edge.EdgeInstance.EdgeID,
			Version:       edge.EdgeInstance.Version,
			BuildHash:     edge.EdgeInstance.BuildHash,
			Metadata:      edge.EdgeInstance.Metadata,
			LastHeartbeat: edge.EdgeInstance.LastHeartbeat,
			Status:        edge.EdgeInstance.Status,
			SessionID:     edge.EdgeInstance.SessionID,
			CreatedAt:     edge.EdgeInstance.CreatedAt,
			UpdatedAt:     edge.EdgeInstance.UpdatedAt,
		},
	}
}

// validateEdgeID validates the edge_id parameter
func validateEdgeID(edgeID string) error {
	if edgeID == "" {
		return helpers.NewBadRequestError("edge_id parameter is required")
	}
	if len(edgeID) > 255 {
		return helpers.NewBadRequestError("edge_id parameter is too long (max 255 characters)")
	}
	return nil
}
