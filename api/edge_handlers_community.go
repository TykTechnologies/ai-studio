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
// CE: Namespace field is always "default"
type EdgeResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		EdgeID        string                 `json:"edge_id"`
		Namespace     string                 `json:"namespace"` // Always "default" in CE
		Version       string                 `json:"version"`
		BuildHash     string                 `json:"build_hash"`
		Metadata      map[string]interface{} `json:"metadata"`
		LastHeartbeat *time.Time             `json:"last_heartbeat"`
		Status        string                 `json:"status"`
		SessionID     string                 `json:"session_id"`
		// Sync status fields for configuration sync tracking
		SyncStatus     string     `json:"sync_status"`
		LoadedChecksum string     `json:"loaded_checksum"`
		LoadedVersion  string     `json:"loaded_version"`
		LastSyncAck    *time.Time `json:"last_sync_ack"`
		CreatedAt      time.Time  `json:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at"`
		EdgeOwner
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
	a.withEdgeOwners(c.Request.Context(), response.Data)

	c.JSON(http.StatusOK, response)
}

// @Summary Get edge instance by ID
// @Description Get details of a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 200 {object} object{data=EdgeResponse}
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
	response := []EdgeResponse{serializeEdgeWithHealth(edge)}
	a.withEdgeOwners(c.Request.Context(), response)
	// Wrapped in "data", as in Enterprise and as the console reads it.
	c.JSON(http.StatusOK, gin.H{"data": response[0]})
}

// @Summary Reload edge configuration
// @Description Trigger a configuration reload for a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 202 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
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

	res, err := a.service.NamespaceService.TriggerEdgeReload(edgeID, initiatedBy(c))
	if err != nil {
		sendPushError(c, err)
		return
	}
	pushAccepted(c, res)
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
	a.listPushes(c)
}

// @Summary Reload all edge gateways
// @Description Trigger a configuration reload for all edge gateways (CE: works for single "default" namespace). 409 when there is no edge to push to.
// @Tags edges
// @Accept json
// @Produce json
// @Success 202 {object} map[string]interface{}
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/reload-all [post]
// @Security BearerAuth
func (a *API) reloadAllEdges(c *gin.Context) {
	// CE: every edge is in the "default" namespace.
	res, err := a.service.NamespaceService.TriggerNamespaceReload(models.DefaultNamespace, initiatedBy(c))
	if err != nil {
		sendPushError(c, err)
		return
	}
	pushAccepted(c, res)
}

// Helper functions

// serializeEdge converts an EdgeInstance model to API response format (CE: namespace always "default")
func serializeEdge(edge *models.EdgeInstance) EdgeResponse {
	namespace := edge.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.ID), 10),
		Attributes: struct {
			EdgeID         string                 `json:"edge_id"`
			Namespace      string                 `json:"namespace"`
			Version        string                 `json:"version"`
			BuildHash      string                 `json:"build_hash"`
			Metadata       map[string]interface{} `json:"metadata"`
			LastHeartbeat  *time.Time             `json:"last_heartbeat"`
			Status         string                 `json:"status"`
			SessionID      string                 `json:"session_id"`
			SyncStatus     string                 `json:"sync_status"`
			LoadedChecksum string                 `json:"loaded_checksum"`
			LoadedVersion  string                 `json:"loaded_version"`
			LastSyncAck    *time.Time             `json:"last_sync_ack"`
			CreatedAt      time.Time              `json:"created_at"`
			UpdatedAt      time.Time              `json:"updated_at"`
			EdgeOwner
		}{
			EdgeID:         edge.EdgeID,
			Namespace:      namespace,
			Version:        edge.Version,
			BuildHash:      edge.BuildHash,
			Metadata:       edge.Metadata,
			LastHeartbeat:  edge.LastHeartbeat,
			Status:         edge.Status,
			SessionID:      edge.SessionID,
			SyncStatus:     edge.SyncStatus,
			LoadedChecksum: edge.LoadedChecksum,
			LoadedVersion:  edge.LoadedVersion,
			LastSyncAck:    edge.LastSyncAck,
			CreatedAt:      edge.CreatedAt,
			UpdatedAt:      edge.UpdatedAt,
			EdgeOwner:      EdgeOwner{OwnerNodeID: edge.OwnerNodeID},
		},
	}
}

// serializeEdgeWithHealth converts an EdgeInstanceWithHealth to API response format (CE: namespace always "default")
func serializeEdgeWithHealth(edge *services.EdgeInstanceWithHealth) EdgeResponse {
	namespace := edge.EdgeInstance.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.EdgeInstance.ID), 10),
		Attributes: struct {
			EdgeID         string                 `json:"edge_id"`
			Namespace      string                 `json:"namespace"`
			Version        string                 `json:"version"`
			BuildHash      string                 `json:"build_hash"`
			Metadata       map[string]interface{} `json:"metadata"`
			LastHeartbeat  *time.Time             `json:"last_heartbeat"`
			Status         string                 `json:"status"`
			SessionID      string                 `json:"session_id"`
			SyncStatus     string                 `json:"sync_status"`
			LoadedChecksum string                 `json:"loaded_checksum"`
			LoadedVersion  string                 `json:"loaded_version"`
			LastSyncAck    *time.Time             `json:"last_sync_ack"`
			CreatedAt      time.Time              `json:"created_at"`
			UpdatedAt      time.Time              `json:"updated_at"`
			EdgeOwner
		}{
			EdgeID:         edge.EdgeInstance.EdgeID,
			Namespace:      namespace,
			Version:        edge.EdgeInstance.Version,
			BuildHash:      edge.EdgeInstance.BuildHash,
			Metadata:       edge.EdgeInstance.Metadata,
			LastHeartbeat:  edge.EdgeInstance.LastHeartbeat,
			Status:         edge.EdgeInstance.Status,
			SessionID:      edge.EdgeInstance.SessionID,
			SyncStatus:     edge.EdgeInstance.SyncStatus,
			LoadedChecksum: edge.EdgeInstance.LoadedChecksum,
			LoadedVersion:  edge.EdgeInstance.LoadedVersion,
			LastSyncAck:    edge.EdgeInstance.LastSyncAck,
			CreatedAt:      edge.EdgeInstance.CreatedAt,
			UpdatedAt:      edge.EdgeInstance.UpdatedAt,
			EdgeOwner:      EdgeOwner{OwnerNodeID: edge.EdgeInstance.OwnerNodeID},
		},
	}
}
