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
	response := serializeEdgeWithHealth(edge)
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

	// Get current user for audit trail
	user, exists := c.Get("user")
	initiatedBy := "unknown"
	if exists {
		if u, ok := user.(*models.User); ok {
			initiatedBy = u.Email
		}
	}

	// Trigger reload via namespace service
	operation, err := a.service.NamespaceService.TriggerEdgeReload(edgeID, initiatedBy)
	if err != nil {
		if err.Error() == "edge not found: failed to get edge: record not found" {
			helpers.SendErrorResponse(c, helpers.NewNotFoundError("Edge instance not found"))
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"data": gin.H{
			"type": "reload-operations",
			"id":   operation.OperationID,
			"attributes": gin.H{
				"operation_id": operation.OperationID,
				"status":       operation.Status,
				"message":      operation.Message,
			},
		},
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
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/edges/reload-operations [get]
// @Security BearerAuth
func (a *API) listReloadOperations(c *gin.Context) {
	// Get reload coordinator from namespace service
	if a.service.NamespaceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Namespace service not available",
		})
		return
	}

	reloadCoordinator := a.service.NamespaceService.GetReloadCoordinator()
	if reloadCoordinator == nil {
		c.JSON(http.StatusOK, gin.H{
			"data":    []interface{}{},
			"message": "Reload coordinator not available (standalone mode)",
		})
		return
	}

	// Get active operations from reload coordinator
	operations := reloadCoordinator.ListActiveOperations()

	// Convert to API response format
	data := make([]gin.H, len(operations))
	for i, op := range operations {
		data[i] = gin.H{
			"type": "reload-operations",
			"id":   op.OperationID,
			"attributes": gin.H{
				"operation_id": op.OperationID,
				"target_edges": op.TargetEdges,
				"initiated_by": op.InitiatedBy,
				"initiated_at": op.InitiatedAt,
				"status":       op.Status,
				"progress":     op.Progress,
				"message":      op.Message,
			},
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
	})
}

// @Summary Reload all edge gateways
// @Description Trigger a configuration reload for all edge gateways (CE: works for single "default" namespace)
// @Tags edges
// @Accept json
// @Produce json
// @Success 202 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/reload-all [post]
// @Security BearerAuth
func (a *API) reloadAllEdges(c *gin.Context) {
	// CE: Reload all edges in "default" namespace (all edges are in default anyway)

	// Get current user for audit trail
	user, exists := c.Get("user")
	initiatedBy := "unknown"
	if exists {
		if u, ok := user.(*models.User); ok {
			initiatedBy = u.Email
		}
	}

	// Trigger namespace reload for "default" (in CE, all edges are in default)
	operation, err := a.service.NamespaceService.TriggerNamespaceReload("default", initiatedBy)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to trigger global reload: "+err.Error()))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"data": gin.H{
			"type": "reload-operations",
			"id":   operation.OperationID,
			"attributes": gin.H{
				"operation_id": operation.OperationID,
				"status":       operation.Status,
				"message":      "Global reload triggered for all edge gateways",
			},
		},
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
