// tests/integration/edge_streaming_test.go
package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
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
			Mode:      "control",
			GRPCPort:  9999, // Use different port for test
			AuthToken: "test-auth-token", // Required for authentication
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
			Mode:              "edge",
			ControlEndpoint:   "localhost:9999",
			EdgeID:            "test-edge-1",
			EdgeNamespace:     "test-namespace",
			ClientToken:       "test-auth-token", // Match control server auth token
			HeartbeatInterval: 100 * time.Millisecond, // Required for heartbeat worker
			AllowInsecure:     true,                   // Enable insecure connections for testing
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

// Enhanced integration tests for bidirectional streaming improvements

func TestSimpleEdgeClient_BidirectionalStreaming(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Setup test config for control server
	controlConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:      "control",
			GRPCPort:  9998, // Different port from other tests
			AuthToken: "test-auth-token", // Required for authentication
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
	time.Sleep(200 * time.Millisecond)

	// Setup edge client
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:              "edge",
			ControlEndpoint:   "localhost:9998",
			ClientToken:       "test-auth-token", // Match control server auth token
			AllowInsecure:     true, // Enable insecure connections for testing
			EdgeID:            "test-edge-streaming",
			EdgeNamespace:     "test-namespace",
			HeartbeatInterval: 100 * time.Millisecond, // Fast heartbeats for testing
		},
	}

	edgeClient := grpc.NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	// Test configuration change callback
	configUpdates := make(chan *pb.ConfigurationSnapshot, 5)
	edgeClient.SetOnConfigChange(func(config *pb.ConfigurationSnapshot) {
		configUpdates <- config
	})

	// Start edge client
	err = edgeClient.Start()
	require.NoError(t, err, "Edge client should start successfully")

	// Verify streaming connection is established
	assert.True(t, edgeClient.IsConnected())

	// Wait for initial configuration
	select {
	case config := <-configUpdates:
		assert.NotNil(t, config)
		assert.NotEmpty(t, config.Version)
		t.Logf("Received initial configuration version: %s", config.Version)
	case <-time.After(2 * time.Second):
		t.Fatal("Should receive initial configuration within 2 seconds")
	}

	// Test full sync request
	err = edgeClient.RequestFullSync()
	assert.NoError(t, err, "Full sync should succeed")

	// Verify configuration update received via stream
	select {
	case config := <-configUpdates:
		assert.NotNil(t, config)
		t.Logf("Received sync configuration version: %s", config.Version)
	case <-time.After(2 * time.Second):
		t.Fatal("Should receive sync configuration within 2 seconds")
	}

	// Cleanup
	edgeClient.Stop()
	controlServer.Stop()
}

func TestSimpleEdgeClient_ErrorHandling(t *testing.T) {
	// Test various error scenarios without starting actual servers
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:            "edge",
			ControlEndpoint: "localhost:99999", // Non-existent port
			EdgeID:          "test-edge-error",
			EdgeNamespace:   "test",
			AllowInsecure:   true, // Enable insecure connections for testing
		},
	}

	edgeClient := grpc.NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	// Test connection failure
	err := edgeClient.Start()
	assert.Error(t, err, "Should fail to connect to non-existent server")
	// The error can be either about registration or connection depending on timing
	assert.True(t,
		strings.Contains(err.Error(), "failed to register") ||
		strings.Contains(err.Error(), "failed to connect") ||
		strings.Contains(err.Error(), "connection error") ||
		strings.Contains(err.Error(), "invalid port"),
		"Expected connection/registration error, got: %v", err)

	// Test token validation without connection
	_, err = edgeClient.ValidateTokenOnDemand("test-token")
	assert.Error(t, err)
	// Error message depends on connection state
	assert.True(t,
		strings.Contains(err.Error(), "not connected") ||
		strings.Contains(err.Error(), "token validation failed"),
		"Expected connection error, got: %v", err)

	// Test configuration retrieval without connection
	config := edgeClient.GetCurrentConfiguration()
	assert.Nil(t, config, "Should not have configuration when not connected")

	// Test full sync without connection
	err = edgeClient.RequestFullSync()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to request full sync")
}

// Note: Message handler tests are implemented in the grpc package unit tests
// since they require access to unexported methods

// TestEdgeSyncService_BudgetUsageSyncOnDBWipe tests that when an edge DB is wiped,
// the budget usage is correctly restored from the control server's snapshot
func TestEdgeSyncService_BudgetUsageSyncOnDBWipe(t *testing.T) {
	// Setup test database for edge (simulating a fresh/wiped DB)
	edgeDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations on the fresh edge DB
	err = database.Migrate(edgeDB)
	require.NoError(t, err)

	namespace := "test-budget-sync"
	now := time.Now()

	// Simulate a configuration snapshot from control server
	// This is what the edge would receive after the DB wipe
	snapshotFromControl := &pb.ConfigurationSnapshot{
		Version:       "budget-test-v1",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "App Exceeded Budget",
				Description:        "App that has exceeded its budget",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      50.0,  // $50 budget
				CurrentPeriodUsage: 75.25, // $75.25 spent - EXCEEDS BUDGET
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
			{
				Id:                 2,
				Name:               "App Within Budget",
				Description:        "App within its budget",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      100.0, // $100 budget
				CurrentPeriodUsage: 30.50, // $30.50 spent
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
			{
				Id:                 3,
				Name:               "App No Budget",
				Description:        "App without budget tracking",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      0, // No budget
				CurrentPeriodUsage: 0, // No usage tracking
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	// Create edge sync service and sync the configuration
	syncService := services.NewEdgeSyncService(edgeDB, namespace)
	err = syncService.SyncConfiguration(snapshotFromControl)
	require.NoError(t, err, "Sync should succeed")

	// Verify apps were synced
	var apps []database.App
	err = edgeDB.Find(&apps).Error
	require.NoError(t, err)
	assert.Len(t, apps, 3, "All 3 apps should be synced")

	// Verify budget usage was initialized from control server
	// Note: TotalCost is stored as dollars * 10000 for consistency with AI Studio format
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// App 1: Exceeded budget - should have BudgetUsage with $75.25 * 10000 = 752500
	var usage1 database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage1).Error
	assert.NoError(t, err, "App 1 should have BudgetUsage record")
	assert.InDelta(t, 752500.0, usage1.TotalCost, 1.0, "App 1 usage should be 752500 (75.25 * 10000)")

	// App 2: Within budget - should have BudgetUsage with $30.50 * 10000 = 305000
	var usage2 database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 2, periodStart).First(&usage2).Error
	assert.NoError(t, err, "App 2 should have BudgetUsage record")
	assert.InDelta(t, 305000.0, usage2.TotalCost, 1.0, "App 2 usage should be 305000 (30.50 * 10000)")

	// App 3: No budget - should NOT have BudgetUsage
	var usage3 database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 3, periodStart).First(&usage3).Error
	assert.Error(t, err, "App 3 should NOT have BudgetUsage record")
	assert.Equal(t, gorm.ErrRecordNotFound, err)

	// Simulate budget check for App 1 (exceeded budget)
	// In a real scenario, the budget service would check this
	var app1BudgetUsage database.BudgetUsage
	app1UsageErr := edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&app1BudgetUsage).Error
	assert.NoError(t, app1UsageErr)

	// Verify budget enforcement would work
	var app1 database.App
	err = edgeDB.First(&app1, 1).Error
	require.NoError(t, err)
	assert.Equal(t, 50.0, app1.MonthlyBudget)

	// The budget is $50, usage is 752500 (stored format) = $75.25 - this should be blocked
	// Convert for comparison: usage1.TotalCost / 10000 > app1.MonthlyBudget
	assert.True(t, (usage1.TotalCost / 10000.0) > app1.MonthlyBudget,
		"App 1 should be over budget: $%.2f spent > $%.2f budget",
		usage1.TotalCost / 10000.0, app1.MonthlyBudget)

	// App 2 should be within budget
	var app2 database.App
	err = edgeDB.First(&app2, 2).Error
	require.NoError(t, err)
	assert.True(t, (usage2.TotalCost / 10000.0) < app2.MonthlyBudget,
		"App 2 should be within budget: $%.2f spent < $%.2f budget",
		usage2.TotalCost / 10000.0, app2.MonthlyBudget)
}

// TestEdgeSyncService_BudgetUsagePreservesExistingRecords tests that syncing
// preserves or updates existing budget usage records correctly
func TestEdgeSyncService_BudgetUsagePreservesExistingRecords(t *testing.T) {
	// Setup test database
	edgeDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(edgeDB)
	require.NoError(t, err)

	namespace := "test-preserve"
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Pre-create budget usage record (simulating edge was running before)
	// Note: TotalCost is stored as dollars * 10000, so $10.00 = 100000
	existingUsage := &database.BudgetUsage{
		AppID:            1,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		TotalCost:        100000.0, // Edge tracked $10.00 (stored as dollars * 10000)
		TokensUsed:       1000,
		RequestsCount:    5,
		PromptTokens:     800,
		CompletionTokens: 200,
	}
	err = edgeDB.Create(existingUsage).Error
	require.NoError(t, err)

	// Sync configuration from control server showing MORE usage
	// This simulates the control server being the source of truth
	// Note: CurrentPeriodUsage comes from control server in dollars
	snapshot := &pb.ConfigurationSnapshot{
		Version:       "preserve-test-v1",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "Test App",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      100.0,
				CurrentPeriodUsage: 45.0, // Control server says $45.00 (in dollars)
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	syncService := services.NewEdgeSyncService(edgeDB, namespace)
	err = syncService.SyncConfiguration(snapshot)
	require.NoError(t, err)

	// Verify that the budget usage was updated to the control server's value
	// Note: $45.00 * 10000 = 450000 (stored format)
	var updatedUsage database.BudgetUsage
	err = edgeDB.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&updatedUsage).Error
	require.NoError(t, err)

	// The control server's value ($45.00 * 10000 = 450000) should be used
	assert.InDelta(t, 450000.0, updatedUsage.TotalCost, 1.0,
		"TotalCost should be updated to control server's value of 450000 ($45.00 * 10000)")

	// Note: Other fields like TokensUsed, RequestsCount may be preserved or reset
	// depending on the implementation. The important thing is TotalCost is correct.
}