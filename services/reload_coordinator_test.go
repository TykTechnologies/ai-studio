package services

import (
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
)

// MockControlServer implements ControlServerInterface for testing
type MockControlServer struct {
	edges          map[string]interface{}
	reloadRequests map[string]*pb.ConfigurationReloadRequest
}

func NewMockControlServer() *MockControlServer {
	return &MockControlServer{
		edges:          make(map[string]interface{}),
		reloadRequests: make(map[string]*pb.ConfigurationReloadRequest),
	}
}

func (m *MockControlServer) GetConnectedEdges() map[string]interface{} {
	return m.edges
}

func (m *MockControlServer) SendReloadRequest(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error {
	m.reloadRequests[edgeID] = reloadReq
	return nil
}

func (m *MockControlServer) SetReloadCoordinator(coordinator interface{}) {
	// No-op for testing
}

func (m *MockControlServer) AddEdge(edgeID, namespace string) {
	m.edges[edgeID] = map[string]interface{}{
		"namespace": namespace,
		"connected": true,
	}
}

func setupReloadCoordinatorTest(t *testing.T) (*ReloadCoordinator, *MockControlServer) {
	mockServer := NewMockControlServer()
	coordinator := NewReloadCoordinator(mockServer)
	return coordinator, mockServer
}

func TestNewReloadCoordinator(t *testing.T) {
	mockServer := NewMockControlServer()

	t.Run("Create new reload coordinator", func(t *testing.T) {
		rc := NewReloadCoordinator(mockServer)
		assert.NotNil(t, rc)
		assert.NotNil(t, rc.controlServer)
		assert.NotNil(t, rc.activeOperations)
		assert.Equal(t, 0, len(rc.activeOperations))
	})
}

func TestInitiateNamespaceReload(t *testing.T) {
	t.Run("Initiate reload for namespace with edges", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		// Add edges to different namespaces
		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "production")
		mockServer.AddEdge("edge-3", "development")

		operation, err := rc.InitiateNamespaceReload("production", "admin@test.com", 60)
		assert.NoError(t, err)
		assert.NotNil(t, operation)
		assert.Equal(t, "production", operation.TargetNamespace)
		assert.Equal(t, "admin@test.com", operation.InitiatedBy)
		assert.Len(t, operation.TargetEdges, 2) // Should include edge-1 and edge-2

		// Verify reload requests were sent
		assert.Len(t, mockServer.reloadRequests, 2)
		assert.Contains(t, mockServer.reloadRequests, "edge-1")
		assert.Contains(t, mockServer.reloadRequests, "edge-2")
	})

	t.Run("Initiate reload for global namespace", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("global-edge-1", "")
		mockServer.AddEdge("global-edge-2", "")
		mockServer.AddEdge("prod-edge", "production")

		operation, err := rc.InitiateNamespaceReload("global", "admin@test.com", 60)
		assert.NoError(t, err)
		assert.Len(t, operation.TargetEdges, 2) // Should include only global edges
	})

	t.Run("Initiate reload for all namespaces", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "development")
		mockServer.AddEdge("edge-3", "")

		operation, err := rc.InitiateNamespaceReload("all", "admin@test.com", 60)
		assert.NoError(t, err)
		assert.Len(t, operation.TargetEdges, 3) // Should include all edges
	})

	t.Run("Initiate reload with no matching edges", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")

		operation, err := rc.InitiateNamespaceReload("development", "admin@test.com", 60)
		assert.Error(t, err)
		assert.Nil(t, operation)
		assert.Contains(t, err.Error(), "no connected edges found")
	})

	t.Run("Initiate reload with no connected edges", func(t *testing.T) {
		rc, _ := setupReloadCoordinatorTest(t)

		operation, err := rc.InitiateNamespaceReload("production", "admin@test.com", 60)
		assert.Error(t, err)
		assert.Nil(t, operation)
		assert.Contains(t, err.Error(), "no connected edges found")
	})
}

func TestInitiateEdgeReload(t *testing.T) {
	t.Run("Initiate reload for specific edges", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "development")
		mockServer.AddEdge("edge-3", "production")

		operation, err := rc.InitiateEdgeReload([]string{"edge-1", "edge-3"}, "admin@test.com", 60)
		assert.NoError(t, err)
		assert.NotNil(t, operation)
		assert.Len(t, operation.TargetEdges, 2)
		assert.Contains(t, operation.TargetEdges, "edge-1")
		assert.Contains(t, operation.TargetEdges, "edge-3")
		assert.Equal(t, "admin@test.com", operation.InitiatedBy)
	})

	t.Run("Initiate reload with disconnected edges", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")

		// Request reload for edges that don't exist
		operation, err := rc.InitiateEdgeReload([]string{"edge-99", "edge-100"}, "admin@test.com", 60)
		assert.Error(t, err)
		assert.Nil(t, operation)
		assert.Contains(t, err.Error(), "no connected edges found")
	})

	t.Run("Initiate reload with mix of connected and disconnected edges", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "production")

		// Request reload for some connected and some disconnected edges
		operation, err := rc.InitiateEdgeReload([]string{"edge-1", "edge-99"}, "admin@test.com", 60)
		assert.NoError(t, err)
		assert.NotNil(t, operation)
		assert.Len(t, operation.TargetEdges, 1) // Only edge-1 is connected
		assert.Contains(t, operation.TargetEdges, "edge-1")
	})
}

func TestGetOperationStatus(t *testing.T) {
	rc, mockServer := setupReloadCoordinatorTest(t)

	// Add edges and initiate reload
	mockServer.AddEdge("edge-1", "production")
	mockServer.AddEdge("edge-2", "production")

	operation, err := rc.InitiateNamespaceReload("production", "admin@test.com", 60)
	assert.NoError(t, err)

	t.Run("Get status of existing operation", func(t *testing.T) {
		status, err := rc.GetOperationStatus(operation.OperationID)
		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, operation.OperationID, status.OperationID)
		assert.Equal(t, "production", status.TargetNamespace)
		assert.Equal(t, "in_progress", status.Status)
	})

	t.Run("Get status of non-existent operation", func(t *testing.T) {
		status, err := rc.GetOperationStatus("non-existent-operation-id")
		assert.Error(t, err)
		assert.Nil(t, status)
		assert.Contains(t, err.Error(), "operation not found")
	})
}

func TestListActiveOperations(t *testing.T) {
	t.Run("List operations with no active operations", func(t *testing.T) {
		rc, _ := setupReloadCoordinatorTest(t)

		operations := rc.ListActiveOperations()
		assert.NotNil(t, operations)
		assert.Len(t, operations, 0)
	})

	t.Run("List operations with active operations", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "production")
		mockServer.AddEdge("edge-3", "development")

		// Initiate two operations with slight time difference to ensure unique IDs
		op1, _ := rc.InitiateNamespaceReload("production", "admin1@test.com", 60)
		time.Sleep(2 * time.Millisecond) // Ensure different timestamp for operation ID
		op2, _ := rc.InitiateEdgeReload([]string{"edge-3"}, "admin2@test.com", 60)

		operations := rc.ListActiveOperations()
		assert.Len(t, operations, 2)

		// Verify both operations are present
		opIDs := make(map[string]bool)
		for _, op := range operations {
			opIDs[op.OperationID] = true
		}
		assert.True(t, opIDs[op1.OperationID])
		assert.True(t, opIDs[op2.OperationID])
	})
}

func TestProcessReloadResponse(t *testing.T) {
	rc, mockServer := setupReloadCoordinatorTest(t)

	mockServer.AddEdge("edge-1", "production")
	mockServer.AddEdge("edge-2", "production")

	operation, _ := rc.InitiateNamespaceReload("production", "admin@test.com", 60)

	t.Run("Process successful reload response", func(t *testing.T) {
		response := &pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-1",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
			Message:     "Configuration reloaded successfully",
		}

		rc.ProcessReloadResponse(response)

		// Check that edge status was updated
		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		edgeStatus := op.EdgeStatus["edge-1"]
		rc.mu.RUnlock()

		assert.Equal(t, pb.ReloadPhase_READY, edgeStatus.CurrentPhase)
		assert.NotNil(t, edgeStatus.Success)
		assert.True(t, *edgeStatus.Success)
		assert.Len(t, edgeStatus.ResponseHistory, 1)
	})

	t.Run("Process failed reload response", func(t *testing.T) {
		response := &pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-2",
			Phase:       pb.ReloadPhase_FAILED,
			Success:     false,
			Message:     "Configuration reload failed",
		}

		rc.ProcessReloadResponse(response)

		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		edgeStatus := op.EdgeStatus["edge-2"]
		rc.mu.RUnlock()

		assert.Equal(t, pb.ReloadPhase_FAILED, edgeStatus.CurrentPhase)
		assert.NotNil(t, edgeStatus.Success)
		assert.False(t, *edgeStatus.Success)
		assert.Equal(t, "Configuration reload failed", edgeStatus.ErrorMessage)
	})

	t.Run("Process response for unknown operation", func(t *testing.T) {
		response := &pb.ConfigurationReloadResponse{
			OperationId: "unknown-operation-id",
			EdgeId:      "edge-1",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
			Message:     "OK",
		}

		// Should not panic, just log warning
		rc.ProcessReloadResponse(response)
	})

	t.Run("Process response for unknown edge", func(t *testing.T) {
		response := &pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "unknown-edge",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
			Message:     "OK",
		}

		// Should not panic, just log warning
		rc.ProcessReloadResponse(response)
	})
}

func TestCalculateProgress(t *testing.T) {
	rc, mockServer := setupReloadCoordinatorTest(t)

	mockServer.AddEdge("edge-1", "production")
	mockServer.AddEdge("edge-2", "production")
	mockServer.AddEdge("edge-3", "production")
	mockServer.AddEdge("edge-4", "production")

	operation, _ := rc.InitiateNamespaceReload("production", "admin@test.com", 60)

	t.Run("Calculate progress with no completions", func(t *testing.T) {
		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		progress := rc.calculateProgress(op)
		rc.mu.RUnlock()

		assert.Equal(t, 0, progress)
	})

	t.Run("Calculate progress with partial completions", func(t *testing.T) {
		// Mark 2 out of 4 edges as complete
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-1",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-2",
			Phase:       pb.ReloadPhase_FAILED,
			Success:     false,
		})

		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		progress := rc.calculateProgress(op)
		rc.mu.RUnlock()

		assert.Equal(t, 50, progress) // 2 out of 4 = 50%
	})

	t.Run("Calculate progress with all completions", func(t *testing.T) {
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-3",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-4",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})

		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		progress := rc.calculateProgress(op)
		rc.mu.RUnlock()

		assert.Equal(t, 100, progress)
	})
}

func TestGenerateStatusMessage(t *testing.T) {
	t.Run("Status message for different statuses", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")
		mockServer.AddEdge("edge-2", "production")
		mockServer.AddEdge("edge-3", "production")

		operation, _ := rc.InitiateNamespaceReload("production", "admin@test.com", 60)

		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		rc.mu.RUnlock()

		// Test initiated status
		assert.Equal(t, "in_progress", op.Status) // Operation starts as in_progress

		// Test with partial progress
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-1",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})

		rc.mu.RLock()
		message := rc.generateStatusMessage(op)
		rc.mu.RUnlock()
		assert.Contains(t, message, "Reload")

		// Complete all edges
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-2",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})
		rc.ProcessReloadResponse(&pb.ConfigurationReloadResponse{
			OperationId: operation.OperationID,
			EdgeId:      "edge-3",
			Phase:       pb.ReloadPhase_READY,
			Success:     true,
		})

		rc.mu.RLock()
		finalMessage := rc.generateStatusMessage(op)
		rc.mu.RUnlock()
		assert.Contains(t, finalMessage, "Reload completed successfully")
	})
}

func TestOperationTimeout(t *testing.T) {
	t.Run("Operation times out after specified duration", func(t *testing.T) {
		rc, mockServer := setupReloadCoordinatorTest(t)

		mockServer.AddEdge("edge-1", "production")

		// Initiate reload with 1 second timeout
		operation, err := rc.InitiateNamespaceReload("production", "admin@test.com", 1)
		assert.NoError(t, err)

		// Wait for timeout plus a bit extra
		time.Sleep(1500 * time.Millisecond)

		// Check that operation status is timed_out
		rc.mu.RLock()
		op := rc.activeOperations[operation.OperationID]
		status := op.Status
		rc.mu.RUnlock()

		assert.Equal(t, "timed_out", status)
	})
}

func TestGetTargetEdgesByNamespace(t *testing.T) {
	rc, mockServer := setupReloadCoordinatorTest(t)

	mockServer.AddEdge("prod-1", "production")
	mockServer.AddEdge("prod-2", "production")
	mockServer.AddEdge("dev-1", "development")
	mockServer.AddEdge("global-1", "")

	t.Run("Get production edges", func(t *testing.T) {
		edges := rc.getTargetEdgesByNamespace("production")
		assert.Len(t, edges, 2)
		assert.Contains(t, edges, "prod-1")
		assert.Contains(t, edges, "prod-2")
	})

	t.Run("Get global edges", func(t *testing.T) {
		edges := rc.getTargetEdgesByNamespace("global")
		assert.Len(t, edges, 1)
		assert.Contains(t, edges, "global-1")
	})

	t.Run("Get all edges", func(t *testing.T) {
		edges := rc.getTargetEdgesByNamespace("all")
		assert.Len(t, edges, 4)
	})
}

func TestValidateEdgesConnected(t *testing.T) {
	rc, mockServer := setupReloadCoordinatorTest(t)

	mockServer.AddEdge("edge-1", "production")
	mockServer.AddEdge("edge-2", "production")

	t.Run("Validate all connected edges", func(t *testing.T) {
		connected := rc.validateEdgesConnected([]string{"edge-1", "edge-2"})
		assert.Len(t, connected, 2)
	})

	t.Run("Validate with disconnected edges", func(t *testing.T) {
		connected := rc.validateEdgesConnected([]string{"edge-1", "edge-99"})
		assert.Len(t, connected, 1)
		assert.Contains(t, connected, "edge-1")
	})

	t.Run("Validate with all disconnected edges", func(t *testing.T) {
		connected := rc.validateEdgesConnected([]string{"edge-99", "edge-100"})
		assert.Len(t, connected, 0)
	})
}
