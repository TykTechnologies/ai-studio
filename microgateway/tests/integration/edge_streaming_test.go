// tests/integration/edge_streaming_test.go
package integration

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSimpleEdgeClient_StreamingConnection(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	
	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Setup test config for control server
	controlConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:     "control",
			GRPCPort: 9999, // Use different port for test
		},
	}

	// Start control server
	controlServer := grpc.NewControlServer(controlConfig, db)
	
	// Start control server in background
	go func() {
		err := controlServer.Start()
		if err != nil {
			t.Logf("Control server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Setup test config for edge client
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:            "edge",
			ControlEndpoint: "localhost:9999",
			EdgeID:          "test-edge-1",
			EdgeNamespace:   "test-namespace",
		},
	}

	// Create and start edge client
	edgeClient := grpc.NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")
	
	err = edgeClient.Start()
	require.NoError(t, err, "Edge client should start successfully with streaming")

	// Verify edge is connected
	assert.True(t, edgeClient.IsConnected(), "Edge should be connected after start")

	// Wait for stream to be established
	time.Sleep(200 * time.Millisecond)

	// Check that control server has the edge registered as connected
	connectedEdges := controlServer.GetConnectedEdges()
	assert.Len(t, connectedEdges, 1, "Control server should have one connected edge")

	edgeInfo, exists := connectedEdges["test-edge-1"]
	require.True(t, exists, "test-edge-1 should be in connected edges")

	edgeInfoMap, ok := edgeInfo.(map[string]interface{})
	require.True(t, ok, "Edge info should be a map")

	assert.Equal(t, "test-edge-1", edgeInfoMap["edge_id"])
	assert.Equal(t, "test-namespace", edgeInfoMap["namespace"])
	// Note: Status might be "registered" or "connected" depending on timing

	// Cleanup
	edgeClient.Stop()
	controlServer.Stop()
}

func TestSimpleEdgeClient_ReloadMessageHandling(t *testing.T) {
	// Setup test config
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:        "test-edge-1",
			EdgeNamespace: "test-namespace",
		},
	}

	edgeClient := grpc.NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")
	
	// Mock reload handler
	reloadRequestReceived := false
	var receivedRequest *pb.ConfigurationReloadRequest
	
	mockHandler := &mockReloadHandler{
		handleFunc: func(req *pb.ConfigurationReloadRequest) {
			reloadRequestReceived = true
			receivedRequest = req
		},
	}
	
	edgeClient.SetReloadHandler(mockHandler)

	// Test reload request handling
	testReq := &pb.ConfigurationReloadRequest{
		OperationId:      "test-op-123",
		TargetNamespace:  "test-namespace",
		InitiatedBy:      "test-user",
		TimeoutSeconds:   300,
		InitiatedAt:      timestamppb.Now(),
	}

	edgeClient.HandleReloadRequest(testReq)

	assert.True(t, reloadRequestReceived, "Reload handler should be called")
	assert.Equal(t, "test-op-123", receivedRequest.OperationId)
	assert.Equal(t, "test-namespace", receivedRequest.TargetNamespace)
}

func TestReloadCoordinator_NamespaceFiltering(t *testing.T) {
	// Setup mock control server
	mockServer := &mockControlServer{
		edges: map[string]interface{}{
			"global-edge": map[string]interface{}{
				"edge_id":   "global-edge",
				"namespace": "",
				"status":    "connected",
			},
			"tenant-a-edge": map[string]interface{}{
				"edge_id":   "tenant-a-edge", 
				"namespace": "tenant-a",
				"status":    "connected",
			},
			"tenant-b-edge": map[string]interface{}{
				"edge_id":   "tenant-b-edge",
				"namespace": "tenant-b", 
				"status":    "connected",
			},
		},
		reloadRequests: make(map[string]*pb.ConfigurationReloadRequest),
	}

	coordinator := services.NewReloadCoordinator(mockServer)

	// Test namespace filtering
	t.Run("Filter by specific namespace", func(t *testing.T) {
		op, err := coordinator.InitiateNamespaceReload("tenant-a", "test-user", 300)
		require.NoError(t, err)
		
		assert.Equal(t, "tenant-a", op.TargetNamespace)
		assert.Contains(t, op.TargetEdges, "tenant-a-edge")
		assert.NotContains(t, op.TargetEdges, "tenant-b-edge")
		assert.NotContains(t, op.TargetEdges, "global-edge") // Global edge not included unless namespace is "all"
	})

	t.Run("Reload all namespaces", func(t *testing.T) {
		op, err := coordinator.InitiateNamespaceReload("all", "test-user", 300)
		require.NoError(t, err)
		
		assert.Equal(t, "all", op.TargetNamespace)
		assert.Len(t, op.TargetEdges, 3) // All edges included
		assert.Contains(t, op.TargetEdges, "global-edge")
		assert.Contains(t, op.TargetEdges, "tenant-a-edge")
		assert.Contains(t, op.TargetEdges, "tenant-b-edge")
	})

	t.Run("Global namespace only", func(t *testing.T) {
		op, err := coordinator.InitiateNamespaceReload("", "test-user", 300)
		require.NoError(t, err)
		
		assert.Equal(t, "", op.TargetNamespace)
		assert.Contains(t, op.TargetEdges, "global-edge")
		assert.NotContains(t, op.TargetEdges, "tenant-a-edge")
		assert.NotContains(t, op.TargetEdges, "tenant-b-edge")
	})
}

// Mock implementations for testing

type mockReloadHandler struct {
	handleFunc func(*pb.ConfigurationReloadRequest)
}

func (m *mockReloadHandler) HandleReloadRequest(req *pb.ConfigurationReloadRequest) {
	if m.handleFunc != nil {
		m.handleFunc(req)
	}
}

type mockControlServer struct {
	edges          map[string]interface{}
	reloadRequests map[string]*pb.ConfigurationReloadRequest
}

func (m *mockControlServer) GetConnectedEdges() map[string]interface{} {
	return m.edges
}

func (m *mockControlServer) SendReloadRequest(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error {
	m.reloadRequests[edgeID] = reloadReq
	return nil
}

func (m *mockControlServer) SetReloadCoordinator(coordinator interface{}) {
	// Mock implementation - do nothing
}