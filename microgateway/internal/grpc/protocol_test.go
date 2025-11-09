// internal/grpc/protocol_test.go
package grpc

import (
	"fmt"
	"strings"
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtocolMessageSerialization(t *testing.T) {
	t.Run("EdgeRegistrationRequest Serialization", func(t *testing.T) {
		original := &pb.EdgeRegistrationRequest{
			EdgeId:        "test-edge-123",
			EdgeNamespace: "test-namespace",
			Version:       "1.0.0",
			BuildHash:     "abc123def456",
			Metadata: map[string]string{
				"region":      "us-east-1",
				"environment": "production",
			},
			Health: &pb.HealthStatus{
				Status:    pb.HealthStatus_HEALTHY,
				Message:   "All systems operational",
				Timestamp: timestamppb.Now(),
			},
		}

		// Serialize
		data, err := proto.Marshal(original)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		// Deserialize
		deserialized := &pb.EdgeRegistrationRequest{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		// Verify fields
		assert.Equal(t, original.EdgeId, deserialized.EdgeId)
		assert.Equal(t, original.EdgeNamespace, deserialized.EdgeNamespace)
		assert.Equal(t, original.Version, deserialized.Version)
		assert.Equal(t, original.BuildHash, deserialized.BuildHash)
		assert.Equal(t, original.Metadata, deserialized.Metadata)
		assert.Equal(t, original.Health.Status, deserialized.Health.Status)
		assert.Equal(t, original.Health.Message, deserialized.Health.Message)
	})

	t.Run("AnalyticsPulse Serialization", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)

		original := &pb.AnalyticsPulse{
			EdgeId:         "analytics-edge",
			EdgeNamespace:  "analytics-test",
			PulseTimestamp: timestamppb.New(now),
			DataFrom:       timestamppb.New(past),
			DataTo:         timestamppb.New(now),
			SequenceNumber: 42,
			AnalyticsEvents: []*pb.AnalyticsEvent{
				{
					RequestId:      "req-1",
					AppId:          1,
					LlmId:          2,
					Endpoint:       "/chat/completions",
					Method:         "POST",
					StatusCode:     200,
					RequestTokens:  50,
					ResponseTokens: 150,
					TotalTokens:    200,
					Cost:           0.008,
					LatencyMs:      1200,
					Timestamp:      timestamppb.New(now),
					ModelName:      "gpt-4",
					Vendor:         "openai",
					RequestBody:    `{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}`,
					ResponseBody:   `{"choices":[{"message":{"content":"response"}}]}`,
				},
			},
			BudgetEvents: []*pb.BudgetUsageEvent{
				{
					AppId:            1,
					LlmId:            2,
					TokensUsed:       200,
					Cost:             0.008,
					RequestsCount:    1,
					PromptTokens:     50,
					CompletionTokens: 150,
					Timestamp:        timestamppb.New(now),
					PeriodStart:      timestamppb.New(past),
					PeriodEnd:        timestamppb.New(now),
				},
			},
			ProxySummaries: []*pb.ProxyLogSummary{
				{
					AppId:              1,
					UserId:             100,
					Vendor:             "openai",
					ResponseCode:       200,
					RequestCount:       1,
					TotalRequestBytes:  150,
					TotalResponseBytes: 300,
					AvgLatencyMs:       1200,
					ErrorCount:         0,
					FirstRequest:       timestamppb.New(past),
					LastRequest:        timestamppb.New(now),
					UniqueModels:       []string{"gpt-4"},
					TotalTokens:        200,
					TotalCost:          0.008,
				},
			},
			IsCompressed:  true,
			TotalRecords:  3,
			DataSizeBytes: 1024,
		}

		// Serialize
		data, err := proto.Marshal(original)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		// Deserialize
		deserialized := &pb.AnalyticsPulse{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		// Verify all fields
		assert.Equal(t, original.EdgeId, deserialized.EdgeId)
		assert.Equal(t, original.EdgeNamespace, deserialized.EdgeNamespace)
		assert.Equal(t, original.SequenceNumber, deserialized.SequenceNumber)
		assert.Equal(t, original.TotalRecords, deserialized.TotalRecords)
		assert.Equal(t, original.IsCompressed, deserialized.IsCompressed)
		assert.Equal(t, original.DataSizeBytes, deserialized.DataSizeBytes)

		// Verify timestamps
		assert.True(t, original.PulseTimestamp.AsTime().Equal(deserialized.PulseTimestamp.AsTime()))
		assert.True(t, original.DataFrom.AsTime().Equal(deserialized.DataFrom.AsTime()))
		assert.True(t, original.DataTo.AsTime().Equal(deserialized.DataTo.AsTime()))

		// Verify nested arrays
		assert.Len(t, deserialized.AnalyticsEvents, 1)
		assert.Len(t, deserialized.BudgetEvents, 1)
		assert.Len(t, deserialized.ProxySummaries, 1)

		// Verify analytics event details
		event := deserialized.AnalyticsEvents[0]
		assert.Equal(t, "req-1", event.RequestId)
		assert.Equal(t, uint32(1), event.AppId)
		assert.Equal(t, uint32(2), event.LlmId)
		assert.Equal(t, "gpt-4", event.ModelName)
		assert.Equal(t, "openai", event.Vendor)
		assert.Equal(t, 0.008, event.Cost)
	})

	t.Run("ConfigurationSnapshot Serialization", func(t *testing.T) {
		original := &pb.ConfigurationSnapshot{
			Version:       "v2.1.0",
			EdgeNamespace: "production",
			SnapshotTime:  timestamppb.Now(),
			Llms: []*pb.LLMConfig{
				{
					Id:             1,
					Name:           "Production GPT-4",
					Slug:           "prod-gpt4",
					Vendor:         "openai",
					Endpoint:       "https://api.openai.com/v1",
					DefaultModel:   "gpt-4",
					MaxTokens:      4096,
					TimeoutSeconds: 30,
					RetryCount:     3,
					IsActive:       true,
					MonthlyBudget:  1000.0,
					RateLimitRpm:   60,
					Namespace:      "production",
					CreatedAt:      timestamppb.Now(),
					UpdatedAt:      timestamppb.Now(),
					AppIds:         []uint32{1, 2, 3},
					FilterIds:      []uint32{1},
					PluginIds:      []uint32{1, 2},
				},
			},
			Apps: []*pb.AppConfig{
				{
					Id:             1,
					Name:           "Production App",
					Description:    "Main production application",
					OwnerEmail:     "admin@example.com",
					IsActive:       true,
					MonthlyBudget:  500.0,
					BudgetResetDay: 1,
					RateLimitRpm:   120,
					Namespace:      "production",
					CreatedAt:      timestamppb.Now(),
					UpdatedAt:      timestamppb.Now(),
					LlmIds:         []uint32{1},
					CredentialIds:  []uint32{1},
					TokenIds:       []uint32{1},
				},
			},
			Filters: []*pb.FilterConfig{
				{
					Id:          1,
					Name:        "Content Filter",
					Description: "Basic content filtering",
					Script:      "function filter(req) { return true; }",
					IsActive:    true,
					OrderIndex:  1,
					Namespace:   "production",
					CreatedAt:   timestamppb.Now(),
					UpdatedAt:   timestamppb.Now(),
					LlmIds:      []uint32{1},
				},
			},
			Plugins: []*pb.PluginConfig{
				{
					Id:          1,
					Name:        "Analytics Plugin",
					Description: "Analytics collection plugin",
					Command:     "./analytics-plugin",
					HookType:    "analytics",
					IsActive:    true,
					Namespace:   "production",
					CreatedAt:   timestamppb.Now(),
					UpdatedAt:   timestamppb.Now(),
					LlmIds:      []uint32{1},
				},
			},
			ModelPrices: []*pb.ModelPriceConfig{
				{
					Id:        1,
					Vendor:    "openai",
					ModelName: "gpt-4",
					Cpt:       0.00003,
					Cpit:      0.00006,
					Currency:  "USD",
					Namespace: "production",
					CreatedAt: timestamppb.Now(),
					UpdatedAt: timestamppb.Now(),
				},
			},
		}

		// Serialize
		data, err := proto.Marshal(original)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		// Deserialize
		deserialized := &pb.ConfigurationSnapshot{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		// Verify main fields
		assert.Equal(t, original.Version, deserialized.Version)
		assert.Equal(t, original.EdgeNamespace, deserialized.EdgeNamespace)

		// Verify nested structures
		assert.Len(t, deserialized.Llms, 1)
		assert.Len(t, deserialized.Apps, 1)
		assert.Len(t, deserialized.Filters, 1)
		assert.Len(t, deserialized.Plugins, 1)
		assert.Len(t, deserialized.ModelPrices, 1)

		// Spot check key fields
		llm := deserialized.Llms[0]
		assert.Equal(t, "prod-gpt4", llm.Slug)
		assert.Equal(t, int32(4096), llm.MaxTokens)
		assert.Equal(t, []uint32{1, 2, 3}, llm.AppIds)

		app := deserialized.Apps[0]
		assert.Equal(t, "Production App", app.Name)
		assert.Equal(t, 500.0, app.MonthlyBudget)
		assert.Equal(t, []uint32{1}, app.LlmIds)

		filter := deserialized.Filters[0]
		assert.Equal(t, "Content Filter", filter.Name)
		assert.Contains(t, filter.Script, "function filter")

		plugin := deserialized.Plugins[0]
		assert.Equal(t, "analytics", plugin.HookType)

		price := deserialized.ModelPrices[0]
		assert.Equal(t, "gpt-4", price.ModelName)
		assert.Equal(t, 0.00003, price.Cpt)
	})
}

func TestLargeMessageHandling(t *testing.T) {
	t.Run("Large Analytics Pulse", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)

		// Create a large pulse with many events
		events := make([]*pb.AnalyticsEvent, 1000)
		for i := 0; i < 1000; i++ {
			events[i] = &pb.AnalyticsEvent{
				RequestId:      fmt.Sprintf("req-%d", i),
				AppId:          uint32(i%10 + 1), // Cycle through app IDs
				LlmId:          1,
				Endpoint:       "/chat/completions",
				Method:         "POST",
				StatusCode:     200,
				RequestTokens:  uint32(50 + i%50),
				ResponseTokens: uint32(100 + i%100),
				TotalTokens:    uint32(150 + i%150),
				Cost:           float64(i%100) * 0.001,
				LatencyMs:      uint32(1000 + i%1000),
				Timestamp:      timestamppb.New(now.Add(time.Duration(i) * time.Second)),
				ModelName:      "gpt-4",
				Vendor:         "openai",
			}
		}

		largePulse := &pb.AnalyticsPulse{
			EdgeId:          "large-pulse-edge",
			EdgeNamespace:   "test",
			PulseTimestamp:  timestamppb.New(now),
			DataFrom:        timestamppb.New(past),
			DataTo:          timestamppb.New(now),
			SequenceNumber:  1,
			AnalyticsEvents: events,
			TotalRecords:    1000,
			IsCompressed:    true,
			DataSizeBytes:   100000, // Estimated size
		}

		// Test serialization of large message
		data, err := proto.Marshal(largePulse)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Logf("Large pulse serialized size: %d bytes", len(data))

		// Test deserialization
		deserialized := &pb.AnalyticsPulse{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		assert.Len(t, deserialized.AnalyticsEvents, 1000)
		assert.Equal(t, largePulse.SequenceNumber, deserialized.SequenceNumber)
		assert.Equal(t, largePulse.TotalRecords, deserialized.TotalRecords)

		// Spot check some events
		assert.Equal(t, "req-0", deserialized.AnalyticsEvents[0].RequestId)
		assert.Equal(t, "req-999", deserialized.AnalyticsEvents[999].RequestId)
	})

	t.Run("Large Configuration Snapshot", func(t *testing.T) {
		// Create configuration with many resources
		llms := make([]*pb.LLMConfig, 100)
		apps := make([]*pb.AppConfig, 50)
		filters := make([]*pb.FilterConfig, 20)

		for i := 0; i < 100; i++ {
			llms[i] = &pb.LLMConfig{
				Id:           uint32(i + 1),
				Name:         fmt.Sprintf("LLM %d", i+1),
				Slug:         fmt.Sprintf("llm-%d", i+1),
				Vendor:       "openai",
				Endpoint:     "https://api.openai.com/v1",
				DefaultModel: "gpt-4",
				IsActive:     true,
				Namespace:    fmt.Sprintf("tenant-%d", i%10),
				CreatedAt:    timestamppb.Now(),
				UpdatedAt:    timestamppb.Now(),
			}
		}

		for i := 0; i < 50; i++ {
			apps[i] = &pb.AppConfig{
				Id:            uint32(i + 1),
				Name:          fmt.Sprintf("App %d", i+1),
				Description:   fmt.Sprintf("Application number %d", i+1),
				OwnerEmail:    fmt.Sprintf("user%d@example.com", i+1),
				IsActive:      true,
				MonthlyBudget: float64(i+1) * 100.0,
				Namespace:     fmt.Sprintf("tenant-%d", i%10),
				CreatedAt:     timestamppb.Now(),
				UpdatedAt:     timestamppb.Now(),
			}
		}

		for i := 0; i < 20; i++ {
			filters[i] = &pb.FilterConfig{
				Id:          uint32(i + 1),
				Name:        fmt.Sprintf("Filter %d", i+1),
				Description: fmt.Sprintf("Content filter number %d", i+1),
				Script:      fmt.Sprintf("function filter%d(req) { return true; }", i+1),
				IsActive:    true,
				OrderIndex:  int32(i + 1),
				Namespace:   fmt.Sprintf("tenant-%d", i%5),
				CreatedAt:   timestamppb.Now(),
				UpdatedAt:   timestamppb.Now(),
			}
		}

		largeConfig := &pb.ConfigurationSnapshot{
			Version:       "v2.1.0-large",
			EdgeNamespace: "test",
			SnapshotTime:  timestamppb.Now(),
			Llms:          llms,
			Apps:          apps,
			Filters:       filters,
		}

		// Test serialization
		data, err := proto.Marshal(largeConfig)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		t.Logf("Large config serialized size: %d bytes", len(data))

		// Test deserialization
		deserialized := &pb.ConfigurationSnapshot{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		assert.Len(t, deserialized.Llms, 100)
		assert.Len(t, deserialized.Apps, 50)
		assert.Len(t, deserialized.Filters, 20)

		// Verify data integrity
		assert.Equal(t, "LLM 1", deserialized.Llms[0].Name)
		assert.Equal(t, "App 50", deserialized.Apps[49].Name)
		assert.Equal(t, "Filter 20", deserialized.Filters[19].Name)
	})
}

func TestMessageValidation_EdgeCases(t *testing.T) {
	t.Run("AnalyticsEvent Field Validation", func(t *testing.T) {
		// Test various field combinations and edge cases
		now := time.Now()

		event := &pb.AnalyticsEvent{
			RequestId:         "test-req",
			AppId:             0, // Edge case: zero app ID
			LlmId:             0, // Edge case: zero LLM ID
			Endpoint:          "",
			Method:            "",
			StatusCode:        0,
			RequestTokens:     0,
			ResponseTokens:    0,
			TotalTokens:       0,
			Cost:              0.0,
			LatencyMs:         0,
			Timestamp:         timestamppb.New(now),
			ErrorMessage:      "",
			RequestSizeBytes:  0,
			ResponseSizeBytes: 0,
			ModelName:         "",
			Vendor:            "",
			RequestBody:       "",
			ResponseBody:      "",
		}

		// Should serialize/deserialize even with minimal data
		data, err := proto.Marshal(event)
		assert.NoError(t, err)

		deserialized := &pb.AnalyticsEvent{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		assert.Equal(t, event.RequestId, deserialized.RequestId)
		assert.Equal(t, event.AppId, deserialized.AppId)
		assert.Equal(t, event.LlmId, deserialized.LlmId)
	})

	t.Run("ConfigurationChange Serialization", func(t *testing.T) {
		change := &pb.ConfigurationChange{
			ChangeType: pb.ConfigurationChange_UPDATE,
			EntityType: pb.ConfigurationChange_LLM,
			EntityId:   42,
			EntityData: `{"id":42,"name":"Updated LLM","slug":"updated-llm","vendor":"anthropic","endpoint":"https://api.anthropic.com","is_active":true}`,
			Namespace:  "test-namespace",
			Timestamp:  timestamppb.Now(),
		}

		// Serialize
		data, err := proto.Marshal(change)
		assert.NoError(t, err)

		// Deserialize
		deserialized := &pb.ConfigurationChange{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		assert.Equal(t, change.ChangeType, deserialized.ChangeType)
		assert.Equal(t, change.EntityType, deserialized.EntityType)
		assert.Equal(t, change.EntityId, deserialized.EntityId)
		assert.Equal(t, change.EntityData, deserialized.EntityData)
		assert.Equal(t, change.Namespace, deserialized.Namespace)
	})

	t.Run("HeartbeatRequest with Metrics", func(t *testing.T) {
		heartbeat := &pb.HeartbeatRequest{
			EdgeId:    "metrics-test-edge",
			SessionId: "session-metrics-123",
			Health: &pb.HealthStatus{
				Status:    pb.HealthStatus_HEALTHY,
				Message:   "All systems operational",
				Timestamp: timestamppb.Now(),
				Metrics: map[string]string{
					"cpu_usage":    "25.5",
					"memory_usage": "512MB",
					"disk_usage":   "75%",
				},
			},
			Metrics: &pb.EdgeMetrics{
				RequestsProcessed: 1000,
				ActiveConnections: 25,
				CpuUsagePercent:   25.5,
				MemoryUsageBytes:  536870912, // 512MB
				UptimeSeconds:     86400,     // 24 hours
				CustomMetrics: map[string]float64{
					"cache_hit_rate":    0.95,
					"avg_response_time": 150.5,
					"error_rate":        0.02,
				},
			},
			Timestamp: timestamppb.Now(),
		}

		// Serialize
		data, err := proto.Marshal(heartbeat)
		assert.NoError(t, err)

		// Deserialize
		deserialized := &pb.HeartbeatRequest{}
		err = proto.Unmarshal(data, deserialized)
		assert.NoError(t, err)

		// Verify all fields
		assert.Equal(t, heartbeat.EdgeId, deserialized.EdgeId)
		assert.Equal(t, heartbeat.SessionId, deserialized.SessionId)
		assert.Equal(t, heartbeat.Health.Status, deserialized.Health.Status)
		assert.Equal(t, heartbeat.Health.Metrics, deserialized.Health.Metrics)

		// Verify metrics
		metrics := deserialized.Metrics
		assert.Equal(t, uint64(1000), metrics.RequestsProcessed)
		assert.Equal(t, uint64(25), metrics.ActiveConnections)
		assert.Equal(t, 25.5, metrics.CpuUsagePercent)
		assert.Equal(t, uint64(536870912), metrics.MemoryUsageBytes)
		assert.Equal(t, uint64(86400), metrics.UptimeSeconds)
		assert.Equal(t, 0.95, metrics.CustomMetrics["cache_hit_rate"])
	})
}

func TestMessageSizeCalculation(t *testing.T) {
	t.Run("Empty vs Populated Message Sizes", func(t *testing.T) {
		// Empty analytics pulse
		emptyPulse := &pb.AnalyticsPulse{
			EdgeId:         "test",
			SequenceNumber: 1,
			TotalRecords:   0,
		}

		emptyData, err := proto.Marshal(emptyPulse)
		assert.NoError(t, err)

		// Populated analytics pulse
		now := time.Now()
		populatedPulse := &pb.AnalyticsPulse{
			EdgeId:         "test",
			EdgeNamespace:  "namespace",
			PulseTimestamp: timestamppb.New(now),
			DataFrom:       timestamppb.New(now.Add(-1 * time.Hour)),
			DataTo:         timestamppb.New(now),
			SequenceNumber: 1,
			AnalyticsEvents: []*pb.AnalyticsEvent{
				{
					RequestId:    "req-1",
					AppId:        1,
					TotalTokens:  100,
					Cost:         0.001,
					Timestamp:    timestamppb.New(now),
					RequestBody:  strings.Repeat("a", 1000), // 1KB request
					ResponseBody: strings.Repeat("b", 2000), // 2KB response
				},
			},
			TotalRecords: 1,
		}

		populatedData, err := proto.Marshal(populatedPulse)
		assert.NoError(t, err)

		t.Logf("Empty pulse: %d bytes", len(emptyData))
		t.Logf("Populated pulse: %d bytes", len(populatedData))

		assert.Greater(t, len(populatedData), len(emptyData),
			"Populated message should be larger than empty message")

		// Test that large request/response bodies significantly increase size
		assert.Greater(t, len(populatedData), 3000,
			"Message with 3KB+ of body data should be substantial")
	})
}
