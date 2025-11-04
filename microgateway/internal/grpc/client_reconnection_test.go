// internal/grpc/client_reconnection_test.go
package grpc

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSimpleEdgeClient_ExponentialBackoff(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:        "test-edge-backoff",
			EdgeNamespace: "test",
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")
	client.reconnectInterval = 1 * time.Second // Base delay

	tests := []struct {
		name               string
		baseDelay          time.Duration
		maxDelay           time.Duration
		multiplier         float64
		jitterFactor       float64
		attempt            int
		expectedMinDelay   time.Duration
		expectedMaxDelay   time.Duration
	}{
		{
			name:             "First attempt",
			baseDelay:        1 * time.Second,
			maxDelay:         60 * time.Second,
			multiplier:       2.0,
			jitterFactor:     0.1,
			attempt:          1,
			expectedMinDelay: 900 * time.Millisecond, // Base - 10% jitter
			expectedMaxDelay: 1100 * time.Millisecond, // Base + 10% jitter
		},
		{
			name:             "Second attempt",
			baseDelay:        1 * time.Second,
			maxDelay:         60 * time.Second,
			multiplier:       2.0,
			jitterFactor:     0.1,
			attempt:          2,
			expectedMinDelay: 1800 * time.Millisecond, // 2s - 10% jitter
			expectedMaxDelay: 2200 * time.Millisecond, // 2s + 10% jitter
		},
		{
			name:             "High attempt capped at max",
			baseDelay:        1 * time.Second,
			maxDelay:         10 * time.Second,
			multiplier:       2.0,
			jitterFactor:     0.1,
			attempt:          10, // Would normally be 512s, but capped
			expectedMinDelay: 9 * time.Second,    // Max - 10% jitter
			expectedMaxDelay: 11 * time.Second,   // Max + 10% jitter (can exceed max due to jitter)
		},
		{
			name:             "No jitter",
			baseDelay:        2 * time.Second,
			maxDelay:         60 * time.Second,
			multiplier:       2.0,
			jitterFactor:     0.0, // No jitter
			attempt:          3,
			expectedMinDelay: 8 * time.Second, // Exactly 2^2 * 2s = 8s
			expectedMaxDelay: 8 * time.Second, // No jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := client.calculateBackoffDelay(tt.baseDelay, tt.maxDelay, tt.multiplier, tt.jitterFactor, tt.attempt)

			assert.GreaterOrEqual(t, delay, tt.expectedMinDelay, "Delay should be at least minimum expected")
			assert.LessOrEqual(t, delay, tt.expectedMaxDelay, "Delay should not exceed maximum expected")

			t.Logf("Attempt %d: delay=%v (expected range: %v-%v)",
				tt.attempt, delay, tt.expectedMinDelay, tt.expectedMaxDelay)
		})
	}
}

func TestSimpleEdgeClient_ErrorCategorization(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID: "test-edge-errors",
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	tests := []struct {
		name             string
		errorMessage     string
		expectedCategory string
	}{
		{
			name:             "Connection Refused",
			errorMessage:     "connection refused",
			expectedCategory: "CONNECTION_REFUSED",
		},
		{
			name:             "Deadline Exceeded",
			errorMessage:     "context deadline exceeded",
			expectedCategory: "TIMEOUT",
		},
		{
			name:             "Context Canceled",
			errorMessage:     "context canceled",
			expectedCategory: "CONTEXT_CANCELED",
		},
		{
			name:             "Transport Closing",
			errorMessage:     "transport is closing",
			expectedCategory: "TRANSPORT_CLOSING",
		},
		{
			name:             "Connection Reset",
			errorMessage:     "connection reset by peer",
			expectedCategory: "CONNECTION_RESET",
		},
		{
			name:             "EOF",
			errorMessage:     "EOF",
			expectedCategory: "EOF",
		},
		{
			name:             "Keepalive Timeout",
			errorMessage:     "keepalive watchdog timeout",
			expectedCategory: "KEEPALIVE_TIMEOUT",
		},
		{
			name:             "Service Unavailable",
			errorMessage:     "rpc error: code = Unavailable",
			expectedCategory: "SERVICE_UNAVAILABLE",
		},
		{
			name:             "Unknown Error",
			errorMessage:     "some unknown error",
			expectedCategory: "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errorMessage)
			category := client.categorizeStreamError(err)
			assert.Equal(t, tt.expectedCategory, category)
		})
	}
}

func TestSimpleEdgeClient_MessageHandlers(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:        "test-edge-messages",
			EdgeNamespace: "test-namespace",
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	t.Run("Registration Response Handler", func(t *testing.T) {
		// Test successful registration
		resp := &pb.EdgeRegistrationResponse{
			Success:   true,
			Message:   "Registration successful",
			SessionId: "session-123",
		}
		err := client.handleRegistrationResponse(resp)
		assert.NoError(t, err)

		// Test failed registration
		resp = &pb.EdgeRegistrationResponse{
			Success: false,
			Message: "Registration failed",
		}
		err = client.handleRegistrationResponse(resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed")
	})

	t.Run("Configuration Update Handler", func(t *testing.T) {
		configReceived := false
		var receivedConfig *pb.ConfigurationSnapshot

		client.SetOnConfigChange(func(config *pb.ConfigurationSnapshot) {
			configReceived = true
			receivedConfig = config
		})

		testConfig := &pb.ConfigurationSnapshot{
			Version:       "v1.0.0",
			EdgeNamespace: "test",
			Llms:          []*pb.LLMConfig{{Id: 1, Name: "Test LLM"}},
			Apps:          []*pb.AppConfig{{Id: 1, Name: "Test App"}},
		}

		err := client.handleConfigurationUpdate(testConfig)
		assert.NoError(t, err)
		assert.True(t, configReceived, "Configuration change callback should be called")
		assert.Equal(t, testConfig, client.GetCurrentConfiguration())
		assert.Equal(t, testConfig, receivedConfig)
	})

	t.Run("Heartbeat Response Handler", func(t *testing.T) {
		// Test normal heartbeat
		resp := &pb.HeartbeatResponse{
			Acknowledged: true,
			Message:      "OK",
		}
		err := client.handleHeartbeatResponse(resp)
		assert.NoError(t, err)

		// Test full sync request (will fail without connection but handler should work)
		resp = &pb.HeartbeatResponse{
			Acknowledged:    true,
			RequestFullSync: true,
		}
		// Skip this test as it would panic without a connection
		// In a real integration test with servers, this would be tested
		// err = client.handleHeartbeatResponse(resp)
		// assert.Error(t, err) // Would fail without gRPC connection
	})

	t.Run("Control Error Handler", func(t *testing.T) {
		// Test non-fatal error
		errMsg := &pb.ErrorMessage{
			Code:    "CONFIG_ERROR",
			Message: "Configuration temporarily unavailable",
			Fatal:   false,
		}
		err := client.handleControlError(errMsg)
		assert.NoError(t, err)

		// Test fatal error
		errMsg = &pb.ErrorMessage{
			Code:    "FATAL_ERROR",
			Message: "Critical system error",
			Fatal:   true,
		}
		err = client.handleControlError(errMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fatal error from control")
	})

	t.Run("Configuration Change Handler", func(t *testing.T) {
		change := &pb.ConfigurationChange{
			ChangeType: pb.ConfigurationChange_UPDATE,
			EntityType: pb.ConfigurationChange_LLM,
			EntityId:   1,
			EntityData: `{"name":"Updated LLM"}`,
			Namespace:  "test",
			Timestamp:  timestamppb.Now(),
		}

		err := client.handleConfigurationChange(change)
		assert.NoError(t, err, "Configuration change should be handled without error")
	})

	t.Run("Reload Request Handler", func(t *testing.T) {
		// Test with no reload handler set
		req := &pb.ConfigurationReloadRequest{
			OperationId:     "test-op-123",
			TargetNamespace: "test-namespace",
		}

		err := client.handleReloadRequest(req)
		assert.NoError(t, err, "Should handle missing reload handler gracefully")

		// Test with mock reload handler
		reloadCalled := false
		var receivedReq *pb.ConfigurationReloadRequest

		mockHandler := &mockReloadHandler{
			handleFunc: func(req *pb.ConfigurationReloadRequest) {
				reloadCalled = true
				receivedReq = req
			},
		}

		client.SetReloadHandler(mockHandler)
		err = client.handleReloadRequest(req)
		assert.NoError(t, err)

		// Verify the handler was called
		time.Sleep(10 * time.Millisecond) // Allow time for async call
		assert.True(t, reloadCalled, "Reload handler should be called")
		assert.Equal(t, req.OperationId, receivedReq.OperationId)
	})
}

func TestSimpleEdgeClient_ProcessControlMessage(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:        "test-edge-processing",
			EdgeNamespace: "test",
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	t.Run("Registration Response Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_RegistrationResponse{
				RegistrationResponse: &pb.EdgeRegistrationResponse{
					Success: true,
					Message: "Test registration",
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err)
	})

	t.Run("Configuration Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_Configuration{
				Configuration: &pb.ConfigurationSnapshot{
					Version:       "v1.0.0",
					EdgeNamespace: "test",
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err)
		assert.Equal(t, "v1.0.0", client.GetCurrentConfiguration().Version)
	})

	t.Run("Change Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_Change{
				Change: &pb.ConfigurationChange{
					ChangeType: pb.ConfigurationChange_CREATE,
					EntityType: pb.ConfigurationChange_APP,
					EntityId:   123,
					Timestamp:  timestamppb.Now(),
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err)
	})

	t.Run("Heartbeat Response Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_HeartbeatResponse{
				HeartbeatResponse: &pb.HeartbeatResponse{
					Acknowledged: true,
					Message:      "Heartbeat OK",
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err)
	})

	t.Run("Error Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_Error{
				Error: &pb.ErrorMessage{
					Code:    "TEST_ERROR",
					Message: "Test error message",
					Fatal:   false,
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err) // Non-fatal errors don't return errors
	})

	t.Run("Reload Request Message", func(t *testing.T) {
		msg := &pb.ControlMessage{
			Message: &pb.ControlMessage_ReloadRequest{
				ReloadRequest: &pb.ConfigurationReloadRequest{
					OperationId:     "test-reload-op",
					TargetNamespace: "test",
					InitiatedBy:     "test-user",
				},
			},
		}

		err := client.processControlMessage(msg)
		assert.NoError(t, err)
	})

	t.Run("Unknown Message Type", func(t *testing.T) {
		// Create a message with no actual message content
		msg := &pb.ControlMessage{}

		err := client.processControlMessage(msg)
		assert.NoError(t, err, "Unknown message types should be handled gracefully")
	})
}

func TestSimpleEdgeClient_ReconnectionAttempts(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:        "test-edge-reconnect",
			EdgeNamespace: "test",
			ControlEndpoint: "localhost:99999", // Non-existent server
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	// Set short intervals for testing
	client.reconnectInterval = 10 * time.Millisecond
	client.maxReconnects = 3

	// Mock a broken connection scenario
	// This will test the logic but won't actually reconnect due to no server

	t.Run("Backoff Calculation Progression", func(t *testing.T) {
		baseDelay := 100 * time.Millisecond
		maxDelay := 5 * time.Second
		multiplier := 2.0
		jitterFactor := 0.1

		// Test that backoff increases properly
		delay1 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, jitterFactor, 1)
		delay2 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, jitterFactor, 2)
		delay3 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, jitterFactor, 3)

		t.Logf("Delays: %v, %v, %v", delay1, delay2, delay3)

		// Generally, delay2 should be larger than delay1 (accounting for jitter)
		// But with jitter, there's some variance, so we test the base calculation
		baseDelay1 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, 0.0, 1)
		baseDelay2 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, 0.0, 2)
		baseDelay3 := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, 0.0, 3)

		assert.Equal(t, baseDelay, baseDelay1)
		assert.Equal(t, 2*baseDelay, baseDelay2)
		assert.Equal(t, 4*baseDelay, baseDelay3)
	})

	t.Run("Max Delay Capping", func(t *testing.T) {
		baseDelay := 1 * time.Second
		maxDelay := 3 * time.Second
		multiplier := 2.0

		// High attempt that would normally exceed max
		delay := client.calculateBackoffDelay(baseDelay, maxDelay, multiplier, 0.0, 10)
		assert.Equal(t, maxDelay, delay, "Delay should be capped at maximum")
	})
}

func TestSimpleEdgeClient_ConnectionManagement(t *testing.T) {
	edgeConfig := &config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:          "test-edge-conn",
			EdgeNamespace:   "test",
			ControlEndpoint: "localhost:99999",
			AllowInsecure:   true, // Enable insecure connections for testing
		},
	}

	client := NewSimpleEdgeClient(edgeConfig, "test", "test-hash", "test-time")

	t.Run("Initial State", func(t *testing.T) {
		assert.False(t, client.IsConnected())
		assert.Nil(t, client.GetCurrentConfiguration())
		assert.Equal(t, "test-edge-conn", client.GetEdgeID())
		assert.Equal(t, "test", client.GetEdgeNamespace())
	})

	t.Run("DialWithKeepalive Configuration", func(t *testing.T) {
		// Test that dial options are configured correctly
		// This tests the method but won't actually connect
		conn, err := client.dialWithKeepalive()

		// The behavior depends on whether AllowInsecure is set
		// If AllowInsecure is false (default), it should error immediately with security message
		// If AllowInsecure is true, grpc.Dial may succeed or fail depending on the endpoint
		if err != nil {
			// Error can be either security-related or connection-related
			assert.True(t,
				strings.Contains(err.Error(), "SECURITY") ||
				strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "connection error"),
				"Expected security or connection error, got: %v", err)
		} else {
			// If no error, connection should be created (will fail on actual use)
			assert.NotNil(t, conn)
			if conn != nil {
				conn.Close()
			}
		}
	})

	t.Run("Connection State Management", func(t *testing.T) {
		// Initially not connected
		assert.False(t, client.connected)
		assert.False(t, client.reconnecting)
		assert.Equal(t, 0, client.reconnectAttempts)

		// Test that SetReloadHandler works
		mockHandler := &mockReloadHandler{}
		client.SetReloadHandler(mockHandler)
		assert.NotNil(t, client.reloadHandler)
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