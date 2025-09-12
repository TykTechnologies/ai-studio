package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// EdgeResponse represents an edge instance in API responses
type EdgeResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		EdgeID        string                 `json:"edge_id"`
		Namespace     string                 `json:"namespace"`
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
// @Description Get a list of registered edge instances with optional namespace filtering
// @Tags edges
// @Accept json
// @Produce json
// @Param namespace query string false "Filter by namespace"
// @Param status query string false "Filter by status (registered, connected, disconnected, unhealthy)"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} EdgeListResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges [get]
// @Security BearerAuth
func (a *API) listEdges(c *gin.Context) {
	// Parse query parameters
	namespace := c.Query("namespace")
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

	// Use edge service to get edges
	edges, totalCount, err := a.service.EdgeService.ListEdges(namespace, status, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Calculate pagination
	totalPages := int(totalCount) / limit
	if int(totalCount)%limit != 0 {
		totalPages++
	}

	// Serialize response
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
	
	edge, err := a.service.EdgeService.GetEdgeByEdgeID(edgeID)
	if err != nil {
		if err.Error() == "failed to get edge: record not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Edge instance not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeEdgeWithHealth(edge)})
}

// @Summary Trigger edge reload
// @Description Trigger a configuration reload on a specific edge instance
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 202 {object} SuccessResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/{edge_id}/reload [post]
// @Security BearerAuth
func (a *API) triggerEdgeReload(c *gin.Context) {
	edgeID := c.Param("edge_id")
	
	// Get current user for audit trail
	user, exists := c.Get("user")
	initiatedBy := "unknown"
	if exists {
		if u, ok := user.(*models.User); ok {
			initiatedBy = u.Email
		}
	}
	
	// Use namespace service to trigger edge reload
	operation, err := a.service.NamespaceService.TriggerEdgeReload(edgeID, initiatedBy)
	if err != nil {
		if err.Error() == "edge not found: failed to get edge: record not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Edge instance not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"data": map[string]interface{}{
			"type": "reload-operations",
			"id":   operation.OperationID,
			"attributes": map[string]interface{}{
				"operation_id":     operation.OperationID,
				"target_namespace": operation.TargetNamespace,
				"target_edges":     operation.TargetEdges,
				"initiated_by":     operation.InitiatedBy,
				"status":           operation.Status,
				"message":          operation.Message,
			},
		},
	})
}

// @Summary Get edge reload operations
// @Description Get list of active reload operations
// @Tags edges
// @Accept json
// @Produce json
// @Success 200 {array} interface{}
// @Router /api/v1/edges/reload-operations [get]
// @Security BearerAuth
func (a *API) listReloadOperations(c *gin.Context) {
	// TODO: Implement reload operation tracking
	c.JSON(http.StatusOK, gin.H{
		"data":    []interface{}{},
		"message": "Reload operation tracking pending implementation",
	})
}

// @Summary Delete edge instance
// @Description Remove an edge instance registration
// @Tags edges
// @Accept json
// @Produce json
// @Param edge_id path string true "Edge ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/edges/{edge_id} [delete]
// @Security BearerAuth
func (a *API) deleteEdge(c *gin.Context) {
	edgeID := c.Param("edge_id")
	
	if err := a.service.EdgeService.DeleteEdge(edgeID); err != nil {
		if err.Error() == "edge not found: record not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Edge instance not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// serializeEdge converts an EdgeInstance model to API response format
func serializeEdge(edge *models.EdgeInstance) EdgeResponse {
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.ID), 10),
		Attributes: struct {
			EdgeID        string                 `json:"edge_id"`
			Namespace     string                 `json:"namespace"`
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
			Namespace:     edge.Namespace,
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

// serializeEdgeWithHealth converts an EdgeInstanceWithHealth to API response format
func serializeEdgeWithHealth(edge *services.EdgeInstanceWithHealth) EdgeResponse {
	return EdgeResponse{
		Type: "edges",
		ID:   strconv.FormatUint(uint64(edge.EdgeInstance.ID), 10),
		Attributes: struct {
			EdgeID        string                 `json:"edge_id"`
			Namespace     string                 `json:"namespace"`
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
			Namespace:     edge.EdgeInstance.Namespace,
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