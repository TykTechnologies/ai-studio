// internal/api/handlers/namespace_handlers.go
package handlers

import (
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
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
		// TODO: Implement edge instance status retrieval
		// This would query the edge_instances table and return current status
		
		c.JSON(http.StatusOK, gin.H{
			"message": "Edge status endpoint - implementation pending",
			"data": []map[string]interface{}{
				{
					"edge_id": "example-edge",
					"namespace": "tenant-a", 
					"status": "connected",
					"version": "v1.0.0-123456",
					"last_heartbeat": time.Now().Format(time.RFC3339),
				},
			},
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

		// TODO: Implement single edge status retrieval
		
		c.JSON(http.StatusOK, gin.H{
			"message": "Single edge status endpoint - implementation pending",
			"data": map[string]interface{}{
				"edge_id": edgeID,
				"status": "connected",
				"version": "v1.0.0-123456",
			},
		})
	}
}