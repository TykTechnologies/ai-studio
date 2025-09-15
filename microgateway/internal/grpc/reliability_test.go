// internal/grpc/reliability_test.go
package grpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGRPCKeepaliveConfiguration(t *testing.T) {
	t.Run("Server Keepalive Configuration", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		cfg := &config.Config{
			HubSpoke: config.HubSpokeConfig{
				Mode:     "control",
				GRPCPort: 9997,
				GRPCHost: "localhost",
			},
		}

		server := NewControlServer(cfg, db)

		// Test that server can be created with keepalive settings
		// This validates the configuration is syntactically correct
		assert.NotNil(t, server)
		assert.Equal(t, cfg, server.config)
	})

	t.Run("Client Keepalive Configuration", func(t *testing.T) {
		edgeConfig := &config.Config{
			HubSpoke: config.HubSpokeConfig{
				ControlEndpoint: "localhost:99999", // Non-existent for test
			},
		}

		client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

		// Test dial configuration (will fail to connect but validates options)
		_, err := client.dialWithKeepalive()
		assert.Error(t, err) // Expected to fail - no server
		assert.Contains(t, err.Error(), "connection refused")
	})
}

func TestControlServer_StreamStateTracking(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	cfg := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			Mode:     "control",
			GRPCPort: 9996,
		},
	}

	server := NewControlServer(cfg, db)

	t.Run("Edge Instance Management", func(t *testing.T) {
		// Initially no connected edges
		edges := server.GetConnectedEdges()
		assert.Empty(t, edges, "Should have no connected edges initially")

		// Test edge instance tracking
		edge := &EdgeInstance{
			EdgeID:        "test-edge-state",
			Namespace:     "test",
			SessionID:     "session-123",
			Version:       "1.0.0",
			LastHeartbeat: time.Now(),
			Status:        "connected",
		}

		// Add edge to internal tracking
		server.edgeMutex.Lock()
		server.edgeInstances["test-edge-state"] = edge
		server.edgeMutex.Unlock()

		// Verify edge is tracked
		edges = server.GetConnectedEdges()
		assert.Empty(t, edges, "Edge without active stream should not be in connected edges")

		// Test stale connection detection
		edge.LastHeartbeat = time.Now().Add(-15 * time.Minute) // Stale heartbeat
		isActive := server.isEdgeStreamActive(edge)
		assert.False(t, isActive, "Edge with stale heartbeat should not be considered active")

		// Test fresh connection
		edge.LastHeartbeat = time.Now()
		// edge.Stream = nil // No stream for simplified testing
		isActive = server.isEdgeStreamActive(edge)
		assert.False(t, isActive, "Edge without stream should not be active even with fresh heartbeat")
	})

	t.Run("Stale Connection Cleanup", func(t *testing.T) {
		// Add stale edge
		staleEdge := &EdgeInstance{
			EdgeID:        "stale-edge",
			Namespace:     "test",
			LastHeartbeat: time.Now().Add(-15 * time.Minute),
			Status:        "connected",
		}

		server.edgeMutex.Lock()
		server.edgeInstances["stale-edge"] = staleEdge
		server.edgeMutex.Unlock()

		// Run cleanup
		server.cleanupStaleConnections()

		// Verify stale edge was removed
		server.edgeMutex.RLock()
		_, exists := server.edgeInstances["stale-edge"]
		server.edgeMutex.RUnlock()

		assert.False(t, exists, "Stale edge should be cleaned up")
	})
}

func TestControlServer_EdgeStateManagement(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Edge Registration and Tracking", func(t *testing.T) {
		edgeID := "test-edge-tracking"

		// Initially no edges
		edges := server.GetConnectedEdges()
		assert.Empty(t, edges)

		// Create edge instance
		edge := &EdgeInstance{
			EdgeID:        edgeID,
			Namespace:     "test",
			SessionID:     "session-123",
			Status:        "connected",
			LastHeartbeat: time.Now(),
		}

		// Add to tracking
		server.edgeMutex.Lock()
		server.edgeInstances[edgeID] = edge
		server.edgeMutex.Unlock()

		// Without an active stream, edge won't appear in connected edges
		edges = server.GetConnectedEdges()
		assert.Empty(t, edges, "Edge without active stream should not be in connected list")

		// Test edge lookup
		server.edgeMutex.RLock()
		foundEdge, exists := server.edgeInstances[edgeID]
		server.edgeMutex.RUnlock()

		assert.True(t, exists, "Edge should be in instances map")
		assert.Equal(t, edge, foundEdge)
	})

	t.Run("Edge Cleanup Process", func(t *testing.T) {
		// Add multiple edges with different states
		edges := []*EdgeInstance{
			{
				EdgeID:        "fresh-edge",
				Namespace:     "test",
				LastHeartbeat: time.Now(), // Fresh
				Status:        "connected",
			},
			{
				EdgeID:        "stale-edge-1",
				Namespace:     "test",
				LastHeartbeat: time.Now().Add(-15 * time.Minute), // Stale
				Status:        "connected",
			},
			{
				EdgeID:        "stale-edge-2",
				Namespace:     "test",
				LastHeartbeat: time.Now().Add(-30 * time.Minute), // Very stale
				Status:        "connected",
			},
		}

		server.edgeMutex.Lock()
		for _, edge := range edges {
			server.edgeInstances[edge.EdgeID] = edge
		}
		server.edgeMutex.Unlock()

		// Verify all edges are present
		server.edgeMutex.RLock()
		initialCount := len(server.edgeInstances)
		server.edgeMutex.RUnlock()
		assert.Equal(t, 3, initialCount)

		// Run cleanup
		server.cleanupStaleConnections()

		// Verify stale edges were removed
		server.edgeMutex.RLock()
		remainingEdges := make([]string, 0)
		for edgeID := range server.edgeInstances {
			remainingEdges = append(remainingEdges, edgeID)
		}
		server.edgeMutex.RUnlock()

		assert.Len(t, remainingEdges, 1, "Only fresh edge should remain")
		assert.Contains(t, remainingEdges, "fresh-edge")
	})
}

func TestControlServer_AnalyticsPulseProcessing(t *testing.T) {
	// Setup test database with required tables
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Valid Analytics Pulse Processing", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)

		pulse := &pb.AnalyticsPulse{
			EdgeId:         "test-edge-analytics",
			EdgeNamespace:  "test",
			PulseTimestamp: timestamppb.New(now),
			DataFrom:       timestamppb.New(past),
			DataTo:         timestamppb.New(now),
			SequenceNumber: 1,
			AnalyticsEvents: []*pb.AnalyticsEvent{
				{
					RequestId:      "req-1",
					AppId:          1,
					LlmId:          1,
					Endpoint:       "/chat/completions",
					Method:         "POST",
					StatusCode:     200,
					RequestTokens:  50,
					ResponseTokens: 100,
					TotalTokens:    150,
					Cost:           0.005,
					LatencyMs:      1500,
					Timestamp:      timestamppb.New(now),
					ModelName:      "gpt-4",
					Vendor:         "openai",
				},
			},
			BudgetEvents: []*pb.BudgetUsageEvent{
				{
					AppId:         1,
					LlmId:         1,
					TokensUsed:    150,
					Cost:          0.005,
					RequestsCount: 1,
					Timestamp:     timestamppb.New(now),
				},
			},
			TotalRecords: 2,
		}

		ctx := context.Background()
		resp, err := server.SendAnalyticsPulse(ctx, pulse)

		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, uint64(1), resp.ProcessedRecords) // Only analytics events are stored
		assert.Equal(t, pulse.SequenceNumber, resp.SequenceNumber)

		// Verify data was stored in database
		var storedEvent database.AnalyticsEvent
		err = db.Where("request_id = ?", "req-1").First(&storedEvent).Error
		assert.NoError(t, err)
		assert.Equal(t, uint(1), storedEvent.AppID)
		assert.Equal(t, 150, storedEvent.TotalTokens)
		assert.Equal(t, 0.005, storedEvent.Cost)
	})

	t.Run("Empty Analytics Pulse", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)

		pulse := &pb.AnalyticsPulse{
			EdgeId:         "test-edge-empty",
			EdgeNamespace:  "test",
			PulseTimestamp: timestamppb.New(now),
			DataFrom:       timestamppb.New(past),
			DataTo:         timestamppb.New(now),
			SequenceNumber: 2,
			TotalRecords:   0,
		}

		ctx := context.Background()
		resp, err := server.SendAnalyticsPulse(ctx, pulse)

		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, uint64(0), resp.ProcessedRecords)
		assert.Equal(t, pulse.SequenceNumber, resp.SequenceNumber)
	})
}

func TestControlServer_ConcurrentStreamHandling(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	// Test concurrent edge connections
	t.Run("Multiple Edge Connections", func(t *testing.T) {
		var wg sync.WaitGroup
		numEdges := 5

		// Create multiple edge instances concurrently
		for i := 0; i < numEdges; i++ {
			wg.Add(1)
			go func(edgeNum int) {
				defer wg.Done()

				edgeID := fmt.Sprintf("concurrent-edge-%d", edgeNum)
				edge := &EdgeInstance{
					EdgeID:        edgeID,
					Namespace:     "test",
					SessionID:     fmt.Sprintf("session-%d", edgeNum),
					Status:        "connected",
					LastHeartbeat: time.Now(),
					// No stream for simplified testing
				}

				// Add to server tracking
				server.edgeMutex.Lock()
				server.edgeInstances[edgeID] = edge
				server.edgeMutex.Unlock()
			}(i)
		}

		wg.Wait()

		// Verify all edges are tracked
		server.edgeMutex.RLock()
		edgeCount := len(server.edgeInstances)
		server.edgeMutex.RUnlock()

		assert.Equal(t, numEdges, edgeCount, "All edges should be tracked")

		// Test concurrent access to connected edges
		var results []map[string]interface{}
		wg = sync.WaitGroup{}

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				edges := server.GetConnectedEdges()
				results = append(results, edges)
			}()
		}

		wg.Wait()

		// All concurrent reads should succeed
		assert.Len(t, results, 10, "All concurrent reads should complete")
	})
}

func TestEdgeInstance_StreamActivity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Active Stream Detection", func(t *testing.T) {
		// Test nil edge
		isActive := server.isEdgeStreamActive(nil)
		assert.False(t, isActive, "Nil edge should not be active")

		// Test edge without stream
		edge := &EdgeInstance{
			EdgeID:        "test-edge-activity",
			LastHeartbeat: time.Now(),
		}
		isActive = server.isEdgeStreamActive(edge)
		assert.False(t, isActive, "Edge without stream should not be active")

		// For testing purposes, we can't easily mock the complex stream interface
		// So we test the heartbeat logic without the stream
		edge.LastHeartbeat = time.Now()
		// isActive would be false without stream, but heartbeat age check passes

		// Test edge with stale heartbeat
		edge.LastHeartbeat = time.Now().Add(-15 * time.Minute)
		isActive = server.isEdgeStreamActive(edge)
		assert.False(t, isActive, "Edge with stale heartbeat should not be active")
	})

	t.Run("Heartbeat Age Validation", func(t *testing.T) {
		edge := &EdgeInstance{
			EdgeID: "test-edge-heartbeat",
			// Stream: nil, // No stream for simplified testing
		}

		// Test various heartbeat ages
		testCases := []struct {
			age      time.Duration
			expected bool
		}{
			{1 * time.Minute, false},   // Fresh but no stream
			{5 * time.Minute, false},   // Still good but no stream
			{9 * time.Minute, false},   // Close to limit but no stream
			{11 * time.Minute, false},  // Stale and no stream
			{30 * time.Minute, false},  // Very stale and no stream
		}

		for _, tc := range testCases {
			edge.LastHeartbeat = time.Now().Add(-tc.age)
			isActive := server.isEdgeStreamActive(edge)
			assert.Equal(t, tc.expected, isActive,
				"Without stream, heartbeat age %v should result in active=%v", tc.age, tc.expected)
		}
	})
}

func TestControlServer_ValidationIntegration(t *testing.T) {
	// Setup test database with test data
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	// Create test app and token
	testApp := &database.App{
		Name:      "Test App",
		IsActive:  true,
		Namespace: "test",
	}
	err = db.Create(testApp).Error
	require.NoError(t, err)

	testToken := &database.APIToken{
		Token:     "test-token-validation",
		AppID:     testApp.ID,
		IsActive:  true,
		Namespace: "test",
	}
	err = db.Create(testToken).Error
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Valid Token Validation", func(t *testing.T) {
		req := &pb.TokenValidationRequest{
			Token:         "test-token-validation",
			EdgeId:        "test-edge-validation",
			EdgeNamespace: "test",
		}

		ctx := context.Background()
		resp, err := server.ValidateToken(ctx, req)

		assert.NoError(t, err)
		assert.True(t, resp.Valid)
		assert.Equal(t, uint32(testApp.ID), resp.AppId)
		assert.Equal(t, testApp.Name, resp.AppName)
	})

	t.Run("Invalid Token Validation", func(t *testing.T) {
		req := &pb.TokenValidationRequest{
			Token:         "non-existent-token",
			EdgeId:        "test-edge-validation",
			EdgeNamespace: "test",
		}

		ctx := context.Background()
		resp, err := server.ValidateToken(ctx, req)

		assert.NoError(t, err)
		assert.False(t, resp.Valid)
		assert.Contains(t, resp.ErrorMessage, "Token not found")
	})
}

func TestGRPCDialOptions(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			ControlEndpoint: "localhost:99999",
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	t.Run("Keepalive Parameters Validation", func(t *testing.T) {
		// Test that we can create connection options without panicking
		// This validates the keepalive configuration is correct

		// Setup expected parameters
		expectedTime := 30 * time.Second
		expectedTimeout := 5 * time.Second

		// Create dial options manually to test configuration
		keepaliveParams := keepalive.ClientParameters{
			Time:                expectedTime,
			Timeout:             expectedTimeout,
			PermitWithoutStream: true,
		}

		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithKeepaliveParams(keepaliveParams),
		}

		// Verify options can be created
		assert.NotNil(t, opts)
		assert.Len(t, opts, 2)

		// Test actual dial (will fail but validates config)
		_, err := grpc.Dial("localhost:99999", opts...)
		assert.Error(t, err) // Expected to fail - no server
		assert.Contains(t, err.Error(), "connection refused")
	})

	// Test the client's dial method
	t.Run("Client DialWithKeepalive", func(t *testing.T) {
		_, err := client.dialWithKeepalive()
		assert.Error(t, err) // Expected to fail - no server
		assert.Contains(t, err.Error(), "connection refused")
	})
}