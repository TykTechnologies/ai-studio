// internal/api/handlers/namespace_handlers.go
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InitiateNamespaceReload initiates a configuration reload for all edges in a namespace
func InitiateNamespaceReload(reloadCoordinator *services.ReloadCoordinator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TargetNamespace string `json:"target_namespace" binding:"required"`
			TimeoutSeconds  int64  `json:"timeout_seconds"`
			InitiatedBy     string `json:"initiated_by"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Set defaults
		if req.TimeoutSeconds <= 0 {
			req.TimeoutSeconds = 300 // 5 minutes default
		}
		if req.InitiatedBy == "" {
			req.InitiatedBy = "api-user"
		}

		// Initiate namespace reload
		operation, err := reloadCoordinator.InitiateNamespaceReload(
			req.TargetNamespace,
			req.InitiatedBy,
			req.TimeoutSeconds,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to initiate namespace reload",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Namespace reload initiated successfully",
			"data": gin.H{
				"operation_id":     operation.OperationID,
				"target_namespace": operation.TargetNamespace,
				"target_edges":     operation.TargetEdges,
				"initiated_by":     operation.InitiatedBy,
				"timeout_at":       operation.TimeoutAt.Format(time.RFC3339),
			},
		})
	}
}

// InitiateEdgeReload initiates a configuration reload for specific edge instances
func InitiateEdgeReload(reloadCoordinator *services.ReloadCoordinator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TargetEdges    []string `json:"target_edges" binding:"required"`
			TimeoutSeconds int64    `json:"timeout_seconds"`
			InitiatedBy    string   `json:"initiated_by"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"message": err.Error(),
			})
			return
		}

		// Set defaults
		if req.TimeoutSeconds <= 0 {
			req.TimeoutSeconds = 300 // 5 minutes default
		}
		if req.InitiatedBy == "" {
			req.InitiatedBy = "api-user"
		}

		// Initiate edge reload
		operation, err := reloadCoordinator.InitiateEdgeReload(
			req.TargetEdges,
			req.InitiatedBy,
			req.TimeoutSeconds,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to initiate edge reload",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Edge reload initiated successfully",
			"data": gin.H{
				"operation_id": operation.OperationID,
				"target_edges": operation.TargetEdges,
				"initiated_by": operation.InitiatedBy,
				"timeout_at":   operation.TimeoutAt.Format(time.RFC3339),
			},
		})
	}
}

// GetReloadOperationStatus returns the status of a reload operation
func GetReloadOperationStatus(reloadCoordinator *services.ReloadCoordinator) gin.HandlerFunc {
	return func(c *gin.Context) {
		operationID := c.Param("operation_id")
		if operationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing operation ID",
				"message": "operation_id parameter is required",
			})
			return
		}

		operation, err := reloadCoordinator.GetOperationStatus(operationID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Operation not found",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": operation,
		})
	}
}

// ListActiveReloadOperations returns all active reload operations
func ListActiveReloadOperations(reloadCoordinator *services.ReloadCoordinator) gin.HandlerFunc {
	return func(c *gin.Context) {
		operations := reloadCoordinator.ListActiveOperations()

		c.JSON(http.StatusOK, gin.H{
			"data": operations,
			"count": len(operations),
		})
	}
}

// GetEdgeInstanceStatus returns status of all connected edge instances
func GetEdgeInstanceStatus(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Query edge instances from database
		var edges []database.EdgeInstance
		query := serviceContainer.DB.Model(&database.EdgeInstance{})
		
		// Optional namespace filtering
		if namespace := c.Query("namespace"); namespace != "" {
			if namespace == "global" {
				query = query.Where("namespace = ''")
			} else {
				query = query.Where("namespace = ?", namespace)
			}
		}
		
		if err := query.Order("last_heartbeat DESC").Find(&edges).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to query edge instances",
				"message": err.Error(),
			})
			return
		}
		
		// Convert to response format
		edgeData := make([]map[string]interface{}, len(edges))
		for i, edge := range edges {
			var lastHeartbeat *string
			if edge.LastHeartbeat != nil {
				hb := edge.LastHeartbeat.Format(time.RFC3339)
				lastHeartbeat = &hb
			}
			
			// Determine connection status based on heartbeat age
			connectionStatus := "disconnected"
			if edge.LastHeartbeat != nil {
				age := time.Since(*edge.LastHeartbeat)
				if age < 5*time.Minute { // Consider connected if heartbeat within 5 minutes
					connectionStatus = "connected"
				} else if age < 15*time.Minute { // Stale if within 15 minutes
					connectionStatus = "stale"
				}
			}
			
			edgeData[i] = map[string]interface{}{
				"edge_id":        edge.EdgeID,
				"namespace":      edge.Namespace,
				"status":         edge.Status,
				"connection":     connectionStatus,
				"version":        edge.Version,
				"build_hash":     edge.BuildHash,
				"session_id":     edge.SessionID,
				"last_heartbeat": lastHeartbeat,
				"created_at":     edge.CreatedAt.Format(time.RFC3339),
				"updated_at":     edge.UpdatedAt.Format(time.RFC3339),
			}
		}
		
		c.JSON(http.StatusOK, gin.H{
			"data": edgeData,
			"total": len(edges),
		})
	}
}

// GetSingleEdgeStatus returns status of a specific edge instance  
func GetSingleEdgeStatus(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		edgeID := c.Param("edge_id")
		if edgeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing edge ID",
				"message": "edge_id parameter is required",
			})
			return
		}

		// Query specific edge instance from database
		var edge database.EdgeInstance
		if err := serviceContainer.DB.Where("edge_id = ?", edgeID).First(&edge).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Edge instance not found",
					"message": fmt.Sprintf("No edge instance found with ID: %s", edgeID),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to query edge instance",
				"message": err.Error(),
			})
			return
		}
		
		// Convert to response format
		var lastHeartbeat *string
		if edge.LastHeartbeat != nil {
			hb := edge.LastHeartbeat.Format(time.RFC3339)
			lastHeartbeat = &hb
		}
		
		// Determine connection status based on heartbeat age
		connectionStatus := "disconnected"
		if edge.LastHeartbeat != nil {
			age := time.Since(*edge.LastHeartbeat)
			if age < 5*time.Minute { // Consider connected if heartbeat within 5 minutes
				connectionStatus = "connected"
			} else if age < 15*time.Minute { // Stale if within 15 minutes
				connectionStatus = "stale"
			}
		}
		
		// Parse metadata if available
		var metadata map[string]interface{}
		if len(edge.Metadata) > 0 {
			json.Unmarshal(edge.Metadata, &metadata)
		}
		
		c.JSON(http.StatusOK, gin.H{
			"data": map[string]interface{}{
				"edge_id":        edge.EdgeID,
				"namespace":      edge.Namespace,
				"status":         edge.Status,
				"connection":     connectionStatus,
				"version":        edge.Version,
				"build_hash":     edge.BuildHash,
				"session_id":     edge.SessionID,
				"last_heartbeat": lastHeartbeat,
				"metadata":       metadata,
				"created_at":     edge.CreatedAt.Format(time.RFC3339),
				"updated_at":     edge.UpdatedAt.Format(time.RFC3339),
			},
		})
	}
}