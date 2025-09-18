// internal/grpc/security_test.go
package grpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestControlServer_Authentication(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	tests := []struct {
		name        string
		authToken   string
		metadata    map[string][]string
		expectError bool
		errorCode   codes.Code
	}{
		{
			name:      "No authentication required",
			authToken: "", // No auth configured
			metadata:  map[string][]string{},
			expectError: false,
		},
		{
			name:      "Valid authentication",
			authToken: "test-secret-token",
			metadata: map[string][]string{
				"authorization": {"Bearer test-secret-token"},
			},
			expectError: false,
		},
		{
			name:      "Missing authorization header",
			authToken: "test-secret-token",
			metadata:  map[string][]string{},
			expectError: true,
			errorCode: codes.Unauthenticated,
		},
		{
			name:      "Invalid token",
			authToken: "test-secret-token",
			metadata: map[string][]string{
				"authorization": {"Bearer wrong-token"},
			},
			expectError: true,
			errorCode: codes.Unauthenticated,
		},
		{
			name:      "Malformed authorization header",
			authToken: "test-secret-token",
			metadata: map[string][]string{
				"authorization": {"InvalidFormat"},
			},
			expectError: true,
			errorCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HubSpoke: config.HubSpokeConfig{
					AuthToken: tt.authToken,
				},
			}

			server := NewControlServer(cfg, db)

			// Create context with metadata
			ctx := context.Background()
			if len(tt.metadata) > 0 {
				md := metadata.New(map[string]string{})
				for key, values := range tt.metadata {
					for _, value := range values {
						md.Append(key, value)
					}
				}
				ctx = metadata.NewIncomingContext(ctx, md)
			}

			err := server.authenticate(ctx)

			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControlServer_NamespaceFiltering(t *testing.T) {
	// Setup test database with namespaced data
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	// Create test tokens in different namespaces
	testApp := &database.App{
		Name:      "Test App",
		IsActive:  true,
		Namespace: "tenant-a",
	}
	err = db.Create(testApp).Error
	require.NoError(t, err)

	globalToken := &database.APIToken{
		Token:     "global-token-123",
		AppID:     testApp.ID,
		IsActive:  true,
		Namespace: "", // Global
	}
	err = db.Create(globalToken).Error
	require.NoError(t, err)

	tenantToken := &database.APIToken{
		Token:     "tenant-a-token-123",
		AppID:     testApp.ID,
		IsActive:  true,
		Namespace: "tenant-a",
	}
	err = db.Create(tenantToken).Error
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	tests := []struct {
		name          string
		token         string
		edgeNamespace string
		expectValid   bool
		errorMsg      string
	}{
		{
			name:          "Global token, global edge",
			token:         "global-token-123",
			edgeNamespace: "",
			expectValid:   true,
		},
		{
			name:          "Global token, tenant edge",
			token:         "global-token-123",
			edgeNamespace: "tenant-a",
			expectValid:   true, // Global tokens visible to all
		},
		{
			name:          "Tenant token, same tenant edge",
			token:         "tenant-a-token-123",
			edgeNamespace: "tenant-a",
			expectValid:   true,
		},
		{
			name:          "Tenant token, global edge",
			token:         "tenant-a-token-123",
			edgeNamespace: "",
			expectValid:   false, // Tenant tokens not visible globally
			errorMsg:      "Token not found or not accessible from this namespace",
		},
		{
			name:          "Tenant token, different tenant edge",
			token:         "tenant-a-token-123",
			edgeNamespace: "tenant-b",
			expectValid:   false, // Cross-tenant access denied
			errorMsg:      "Token not found or not accessible from this namespace",
		},
		{
			name:          "Non-existent token",
			token:         "non-existent-token",
			edgeNamespace: "tenant-a",
			expectValid:   false,
			errorMsg:      "Token not found or not accessible from this namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.TokenValidationRequest{
				Token:         tt.token,
				EdgeId:        "test-edge-namespace",
				EdgeNamespace: tt.edgeNamespace,
			}

			ctx := context.Background()
			resp, err := server.ValidateToken(ctx, req)

			assert.NoError(t, err, "ValidateToken should not return gRPC errors")
			assert.Equal(t, tt.expectValid, resp.Valid)

			if !tt.expectValid && tt.errorMsg != "" {
				assert.Contains(t, resp.ErrorMessage, tt.errorMsg)
			}

			if tt.expectValid {
				assert.Equal(t, uint32(testApp.ID), resp.AppId)
				assert.Equal(t, testApp.Name, resp.AppName)
			}
		})
	}
}

func BenchmarkControlServer_TokenValidation(b *testing.B) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = database.Migrate(db)
	require.NoError(b, err)

	// Create test data
	testApp := &database.App{
		Name:      "Benchmark App",
		IsActive:  true,
		Namespace: "benchmark",
	}
	err = db.Create(testApp).Error
	require.NoError(b, err)

	// Create multiple tokens for benchmarking
	tokens := make([]*database.APIToken, 100)
	for i := 0; i < 100; i++ {
		tokens[i] = &database.APIToken{
			Token:     fmt.Sprintf("benchmark-token-%d", i),
			AppID:     testApp.ID,
			IsActive:  true,
			Namespace: "benchmark",
		}
		err = db.Create(tokens[i]).Error
		require.NoError(b, err)
	}

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	// Benchmark token validation
	b.ResetTimer()
	b.RunParallel(func(pbench *testing.PB) {
		i := 0
		for pbench.Next() {
			tokenIndex := i % 100
			req := &pb.TokenValidationRequest{
				Token:         tokens[tokenIndex].Token,
				EdgeId:        "benchmark-edge",
				EdgeNamespace: "benchmark",
			}

			ctx := context.Background()
			resp, err := server.ValidateToken(ctx, req)

			if err != nil {
				b.Errorf("Token validation failed: %v", err)
			}
			if !resp.Valid {
				b.Errorf("Token should be valid: %s", resp.ErrorMessage)
			}

			i++
		}
	})
}

func BenchmarkAnalyticsPulseProcessing(b *testing.B) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = database.Migrate(db)
	require.NoError(b, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	// Create a sample analytics pulse
	now := time.Now()
	past := now.Add(-1 * time.Hour)

	createPulse := func(sequenceNum uint64, eventCount int) *pb.AnalyticsPulse {
		events := make([]*pb.AnalyticsEvent, eventCount)
		for i := 0; i < eventCount; i++ {
			events[i] = &pb.AnalyticsEvent{
				RequestId:      fmt.Sprintf("bench-req-%d-%d", sequenceNum, i),
				AppId:          uint32(i%10 + 1),
				LlmId:          1,
				Endpoint:       "/chat/completions",
				Method:         "POST",
				StatusCode:     200,
				RequestTokens:  uint32(50 + i%50),
				ResponseTokens: uint32(100 + i%100),
				TotalTokens:    uint32(150 + i%150),
				Cost:           float64(i%100) * 0.001,
				LatencyMs:      uint32(1000 + i%1000),
				Timestamp:      timestamppb.New(now),
				ModelName:      "gpt-4",
				Vendor:         "openai",
			}
		}

		return &pb.AnalyticsPulse{
			EdgeId:          "benchmark-edge",
			EdgeNamespace:   "benchmark",
			PulseTimestamp:  timestamppb.New(now),
			DataFrom:        timestamppb.New(past),
			DataTo:          timestamppb.New(now),
			SequenceNumber:  sequenceNum,
			AnalyticsEvents: events,
			TotalRecords:    uint32(eventCount),
		}
	}

	b.Run("Small Pulse (10 events)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pulse := createPulse(uint64(i), 10)
			ctx := context.Background()

			resp, err := server.SendAnalyticsPulse(ctx, pulse)
			if err != nil {
				b.Errorf("Analytics pulse processing failed: %v", err)
			}
			if !resp.Success {
				b.Errorf("Analytics pulse should succeed: %s", resp.Message)
			}
		}
	})

	b.Run("Medium Pulse (100 events)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pulse := createPulse(uint64(i), 100)
			ctx := context.Background()

			resp, err := server.SendAnalyticsPulse(ctx, pulse)
			if err != nil {
				b.Errorf("Analytics pulse processing failed: %v", err)
			}
			if !resp.Success {
				b.Errorf("Analytics pulse should succeed: %s", resp.Message)
			}
		}
	})

	b.Run("Large Pulse (1000 events)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pulse := createPulse(uint64(i), 1000)
			ctx := context.Background()

			resp, err := server.SendAnalyticsPulse(ctx, pulse)
			if err != nil {
				b.Errorf("Analytics pulse processing failed: %v", err)
			}
			if !resp.Success {
				b.Errorf("Analytics pulse should succeed: %s", resp.Message)
			}
		}
	})
}

func TestControlServer_InputSanitization(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Edge Registration with Special Characters", func(t *testing.T) {
		testCases := []struct {
			name      string
			edgeID    string
			namespace string
			version   string
			expectErr bool
		}{
			{
				name:      "Normal ASCII characters",
				edgeID:    "edge-123-test",
				namespace: "tenant-prod",
				version:   "v1.0.0",
				expectErr: false,
			},
			{
				name:      "Unicode characters",
				edgeID:    "edge-测试-123",
				namespace: "租户-a",
				version:   "v1.0.0-βeta",
				expectErr: false, // Should be accepted
			},
			{
				name:      "Special symbols",
				edgeID:    "edge@#$%^&*()",
				namespace: "tenant!@#",
				version:   "v1.0.0+build",
				expectErr: false, // Should be accepted
			},
			{
				name:      "Very long strings",
				edgeID:    strings.Repeat("x", 100), // Too long
				namespace: "tenant",
				version:   "v1.0.0",
				expectErr: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := &pb.EdgeRegistrationRequest{
					EdgeId:        tc.edgeID,
					EdgeNamespace: tc.namespace,
					Version:       tc.version,
					Health: &pb.HealthStatus{
						Status: pb.HealthStatus_HEALTHY,
					},
				}

				err := server.validateEdgeRegistrationRequest(req)

				if tc.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("Token Validation with Special Characters", func(t *testing.T) {
		testCases := []struct {
			name      string
			token     string
			expectErr bool
		}{
			{
				name:      "Normal token",
				token:     "abc123def456ghi789",
				expectErr: false,
			},
			{
				name:      "Token with special chars",
				token:     "token-with-dashes_and_underscores.and.dots",
				expectErr: false,
			},
			{
				name:      "Very long token",
				token:     strings.Repeat("x", 200), // Too long
				expectErr: true,
			},
			{
				name:      "Empty token",
				token:     "",
				expectErr: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tokenReq := &pb.TokenValidationRequest{
					Token:         tc.token,
					EdgeId:        "test-edge",
					EdgeNamespace: "test",
				}

				err := server.validateTokenValidationRequest(tokenReq)

				if tc.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

func TestControlServer_ConcurrentOperations(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Concurrent Edge Registration", func(t *testing.T) {
		numEdges := 10
		results := make(chan error, numEdges)

		for i := 0; i < numEdges; i++ {
			go func(edgeNum int) {
				req := &pb.EdgeRegistrationRequest{
					EdgeId:        fmt.Sprintf("concurrent-edge-%d", edgeNum),
					EdgeNamespace: "test",
					Version:       "1.0.0",
					Health: &pb.HealthStatus{
						Status:    pb.HealthStatus_HEALTHY,
						Timestamp: timestamppb.Now(),
					},
				}

				ctx := context.Background()
				_, err := server.RegisterEdge(ctx, req)
				results <- err
			}(i)
		}

		// Collect results
		for i := 0; i < numEdges; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent edge registration should succeed")
		}

		// Verify all edges were registered
		server.edgeMutex.RLock()
		edgeCount := len(server.edgeInstances)
		server.edgeMutex.RUnlock()

		assert.Equal(t, numEdges, edgeCount, "All edges should be registered")
	})

	t.Run("Concurrent Analytics Pulse Processing", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)

		numPulses := 5
		results := make(chan error, numPulses)

		for i := 0; i < numPulses; i++ {
			go func(pulseNum int) {
				pulse := &pb.AnalyticsPulse{
					EdgeId:         fmt.Sprintf("pulse-edge-%d", pulseNum),
					EdgeNamespace:  "test",
					PulseTimestamp: timestamppb.New(now),
					DataFrom:       timestamppb.New(past),
					DataTo:         timestamppb.New(now),
					SequenceNumber: uint64(pulseNum),
					AnalyticsEvents: []*pb.AnalyticsEvent{
						{
							RequestId:   fmt.Sprintf("req-%d", pulseNum),
							AppId:       uint32(pulseNum % 3 + 1),
							TotalTokens: uint32(100 + pulseNum),
							Cost:        float64(pulseNum) * 0.001,
							Timestamp:   timestamppb.New(now),
						},
					},
					TotalRecords: 1,
				}

				ctx := context.Background()
				_, err := server.SendAnalyticsPulse(ctx, pulse)
				results <- err
			}(i)
		}

		// Collect results
		for i := 0; i < numPulses; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent analytics pulse processing should succeed")
		}

		// Verify events were stored
		var eventCount int64
		db.Model(&database.AnalyticsEvent{}).Count(&eventCount)
		assert.Equal(t, int64(numPulses), eventCount, "All analytics events should be stored")
	})
}

func TestControlServer_ResourceLimits(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	t.Run("Edge Instance Memory Usage", func(t *testing.T) {
		// Add many edge instances to test memory usage
		for i := 0; i < 1000; i++ {
			edge := &EdgeInstance{
				EdgeID:        fmt.Sprintf("memory-test-edge-%d", i),
				Namespace:     fmt.Sprintf("tenant-%d", i%10),
				SessionID:     fmt.Sprintf("session-%d", i),
				Version:       "1.0.0",
				BuildHash:     "test-hash",
				LastHeartbeat: time.Now(),
				Status:        "connected",
				Metadata: map[string]string{
					"region":      "us-east-1",
					"environment": "test",
				},
			}

			server.edgeMutex.Lock()
			server.edgeInstances[edge.EdgeID] = edge
			server.edgeMutex.Unlock()
		}

		// Verify all edges are tracked
		server.edgeMutex.RLock()
		edgeCount := len(server.edgeInstances)
		server.edgeMutex.RUnlock()

		assert.Equal(t, 1000, edgeCount, "All 1000 edges should be tracked")

		// Test cleanup of old edges
		// Make half of them stale
		server.edgeMutex.Lock()
		for i := 0; i < 500; i++ {
			edgeID := fmt.Sprintf("memory-test-edge-%d", i)
			if edge, exists := server.edgeInstances[edgeID]; exists {
				edge.LastHeartbeat = time.Now().Add(-15 * time.Minute)
			}
		}
		server.edgeMutex.Unlock()

		// Run cleanup
		server.cleanupStaleConnections()

		// Verify stale edges were removed
		server.edgeMutex.RLock()
		finalCount := len(server.edgeInstances)
		server.edgeMutex.RUnlock()

		assert.Equal(t, 500, finalCount, "500 stale edges should be cleaned up")
	})
}

func TestSimpleEdgeClient_SecurityValidation(t *testing.T) {
	t.Run("Secure Connection Configuration", func(t *testing.T) {
		edgeConfig := &config.Config{
			HubSpoke: config.HubSpokeConfig{
				ControlEndpoint: "localhost:99999",
			},
		}

		client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

		// Test that client uses insecure credentials for testing
		// In production, this should be replaced with proper TLS
		_, err := client.dialWithKeepalive()
		assert.Error(t, err) // Expected to fail - no server
		// The error should be connection-related, not TLS-related
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("Token Validation Security", func(t *testing.T) {
		edgeConfig := &config.Config{
			HubSpoke: config.HubSpokeConfig{
				EdgeID:          "security-test-edge",
				EdgeNamespace:   "security-test",
			},
		}

		client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

		// Test with various token formats
		testTokens := []string{
			"normal-token-123",
			"token.with.dots",
			"token_with_underscores",
			"token-with-dashes",
			strings.Repeat("x", 64), // Long but valid
		}

		for _, token := range testTokens {
			// This will fail due to no connection, but tests the request formation
			_, err := client.ValidateTokenOnDemand(token)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not connected to control instance")
		}
	})
}