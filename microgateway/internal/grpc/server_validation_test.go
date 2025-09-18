// internal/grpc/server_validation_test.go
package grpc

import (
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidateEdgeRegistrationRequest(t *testing.T) {
	// Setup test server
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	tests := []struct {
		name        string
		request     *pb.EdgeRegistrationRequest
		expectError bool
		errorCode   codes.Code
		errorMsg    string
	}{
		{
			name: "Valid registration request",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge-123",
				EdgeNamespace: "test-namespace",
				Version:       "1.0.0",
				BuildHash:     "abc123",
				Health: &pb.HealthStatus{
					Status:    pb.HealthStatus_HEALTHY,
					Message:   "Healthy",
					Timestamp: timestamppb.Now(),
				},
				Metadata: map[string]string{
					"region": "us-east-1",
				},
			},
			expectError: false,
		},
		{
			name: "Missing edge_id",
			request: &pb.EdgeRegistrationRequest{
				EdgeNamespace: "test-namespace",
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id is required",
		},
		{
			name: "Edge_id too long",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        strings.Repeat("x", 65), // 65 characters
				EdgeNamespace: "test",
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id must be 64 characters or less",
		},
		{
			name: "Namespace too long",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: strings.Repeat("x", 65), // 65 characters
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_namespace must be 64 characters or less",
		},
		{
			name: "Missing version",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "version is required",
		},
		{
			name: "Version too long",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Version:       strings.Repeat("x", 33), // 33 characters
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "version must be 32 characters or less",
		},
		{
			name: "Missing health status",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Version:       "1.0.0",
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "health status is required",
		},
		{
			name: "Too many metadata entries",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
				Metadata: map[string]string{
					"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5",
					"k6": "v6", "k7": "v7", "k8": "v8", "k9": "v9", "k10": "v10",
					"k11": "v11", // 11 entries
				},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "metadata cannot have more than 10 entries",
		},
		{
			name: "Metadata key too long",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
				Metadata: map[string]string{
					strings.Repeat("x", 65): "value", // 65-char key
				},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "metadata key must be 64 characters or less",
		},
		{
			name: "Metadata value too long",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "valid-edge",
				EdgeNamespace: "test",
				Version:       "1.0.0",
				Health:        &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY},
				Metadata: map[string]string{
					"key": strings.Repeat("x", 257), // 257-char value
				},
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "metadata value must be 256 characters or less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.validateEdgeRegistrationRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Equal(t, tt.errorMsg, st.Message())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTokenValidationRequest(t *testing.T) {
	// Setup test server
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	tests := []struct {
		name        string
		request     *pb.TokenValidationRequest
		expectError bool
		errorCode   codes.Code
		errorMsg    string
	}{
		{
			name: "Valid token validation request",
			request: &pb.TokenValidationRequest{
				Token:         "valid-token-123",
				EdgeId:        "edge-123",
				EdgeNamespace: "test-namespace",
			},
			expectError: false,
		},
		{
			name: "Missing token",
			request: &pb.TokenValidationRequest{
				EdgeId:        "edge-123",
				EdgeNamespace: "test",
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "token is required",
		},
		{
			name: "Token too long",
			request: &pb.TokenValidationRequest{
				Token:         strings.Repeat("x", 129), // 129 characters
				EdgeId:        "edge-123",
				EdgeNamespace: "test",
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "token must be 128 characters or less",
		},
		{
			name: "Missing edge_id",
			request: &pb.TokenValidationRequest{
				Token:         "valid-token",
				EdgeNamespace: "test",
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id is required",
		},
		{
			name: "Edge_id too long",
			request: &pb.TokenValidationRequest{
				Token:         "valid-token",
				EdgeId:        strings.Repeat("x", 65), // 65 characters
				EdgeNamespace: "test",
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id must be 64 characters or less",
		},
		{
			name: "Edge_namespace too long",
			request: &pb.TokenValidationRequest{
				Token:         "valid-token",
				EdgeId:        "edge-123",
				EdgeNamespace: strings.Repeat("x", 65), // 65 characters
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_namespace must be 64 characters or less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.validateTokenValidationRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Equal(t, tt.errorMsg, st.Message())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAnalyticsPulseRequest(t *testing.T) {
	// Setup test server
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name        string
		request     *pb.AnalyticsPulse
		expectError bool
		errorCode   codes.Code
		errorMsg    string
	}{
		{
			name: "Valid analytics pulse",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				DataTo:         timestamppb.New(now),
				SequenceNumber: 1,
				AnalyticsEvents: []*pb.AnalyticsEvent{
					{RequestId: "req-1", AppId: 1, TotalTokens: 100},
				},
				TotalRecords: 1,
			},
			expectError: false,
		},
		{
			name: "Missing edge_id",
			request: &pb.AnalyticsPulse{
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				DataTo:         timestamppb.New(now),
				TotalRecords:   0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id is required",
		},
		{
			name: "Edge_id too long",
			request: &pb.AnalyticsPulse{
				EdgeId:         strings.Repeat("x", 65),
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				DataTo:         timestamppb.New(now),
				TotalRecords:   0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "edge_id must be 64 characters or less",
		},
		{
			name: "Missing pulse_timestamp",
			request: &pb.AnalyticsPulse{
				EdgeId:        "edge-123",
				EdgeNamespace: "test",
				DataFrom:      timestamppb.New(past),
				DataTo:        timestamppb.New(now),
				TotalRecords:  0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "pulse_timestamp is required",
		},
		{
			name: "Missing data_from",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataTo:         timestamppb.New(now),
				TotalRecords:   0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "data_from is required",
		},
		{
			name: "Missing data_to",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				TotalRecords:   0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "data_to is required",
		},
		{
			name: "Invalid time range (data_from after data_to)",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(future), // After data_to
				DataTo:         timestamppb.New(now),
				TotalRecords:   0,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "data_from must be before data_to",
		},
		{
			name: "Too many records",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				DataTo:         timestamppb.New(now),
				AnalyticsEvents: func() []*pb.AnalyticsEvent {
					events := make([]*pb.AnalyticsEvent, 10001) // Too many
					for i := range events {
						events[i] = &pb.AnalyticsEvent{RequestId: "req"}
					}
					return events
				}(),
				TotalRecords: 10001,
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "pulse cannot contain more than 10000 total records",
		},
		{
			name: "Mismatched total_records count",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-123",
				EdgeNamespace:  "test",
				PulseTimestamp: timestamppb.New(now),
				DataFrom:       timestamppb.New(past),
				DataTo:         timestamppb.New(now),
				AnalyticsEvents: []*pb.AnalyticsEvent{
					{RequestId: "req-1"},
					{RequestId: "req-2"},
				},
				TotalRecords: 5, // Says 5 but actually 2
			},
			expectError: true,
			errorCode:   codes.InvalidArgument,
			errorMsg:    "total_records field does not match actual record count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.validateAnalyticsPulseRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Equal(t, tt.errorMsg, st.Message())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAnalyticsPulseRequest_CombinedRecordTypes(t *testing.T) {
	// Setup test server
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	cfg := &config.Config{}
	server := NewControlServer(cfg, db)

	now := time.Now()
	past := now.Add(-1 * time.Hour)

	// Test with mixed record types
	request := &pb.AnalyticsPulse{
		EdgeId:         "edge-123",
		EdgeNamespace:  "test",
		PulseTimestamp: timestamppb.New(now),
		DataFrom:       timestamppb.New(past),
		DataTo:         timestamppb.New(now),
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "req-1"},
			{RequestId: "req-2"},
		},
		BudgetEvents: []*pb.BudgetUsageEvent{
			{AppId: 1, TokensUsed: 100},
		},
		ProxySummaries: []*pb.ProxyLogSummary{
			{AppId: 1, RequestCount: 5},
			{AppId: 2, RequestCount: 3},
		},
		TotalRecords: 5, // 2 + 1 + 2 = 5
	}

	err = server.validateAnalyticsPulseRequest(request)
	assert.NoError(t, err, "Mixed record types should validate correctly")

	// Test with incorrect total
	request.TotalRecords = 3 // Wrong count
	err = server.validateAnalyticsPulseRequest(request)
	assert.Error(t, err)

	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "total_records field does not match actual record count", st.Message())
}