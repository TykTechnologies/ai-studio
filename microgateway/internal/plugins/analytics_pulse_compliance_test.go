package plugins

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsPulsePlugin_BufferComplianceEvents(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{
			MaxBufferSize:          10000,
			IncludeProxySummaries:  true,
			CompressionEnabled:     false,
			TimeoutSeconds:         5,
		},
		edgeID:        "test-edge",
		edgeNamespace: "test",
	}

	events := []ComplianceEventBuffer{
		{
			AppID:       1,
			UserID:      10,
			LLMID:       5,
			FilterName:  "pii-filter",
			FilterScope: "proxy_request",
			EventType:   "pii_redacted",
			Severity:    "warning",
			Description: "SSN found",
			Metadata:    `{"count":3}`,
			Vendor:      "openai",
			ModelName:   "gpt-4",
			Timestamp:   time.Now(),
		},
		{
			AppID:       2,
			UserID:      20,
			LLMID:       8,
			FilterName:  "tone-filter",
			FilterScope: "chat_request",
			EventType:   "content_rewritten",
			Severity:    "info",
			Description: "Tone adjusted",
			Vendor:      "anthropic",
			ModelName:   "claude-3",
			Timestamp:   time.Now(),
		},
	}

	// Buffer events
	plugin.BufferComplianceEvents(events)

	// Verify buffer contents
	plugin.bufferMutex.RLock()
	assert.Len(t, plugin.complianceBuffer, 2)
	assert.Equal(t, "pii_redacted", plugin.complianceBuffer[0].EventType)
	assert.Equal(t, "content_rewritten", plugin.complianceBuffer[1].EventType)
	plugin.bufferMutex.RUnlock()

	// Buffer more events
	plugin.BufferComplianceEvents([]ComplianceEventBuffer{
		{
			AppID:     3,
			EventType: "silent_failure",
			Severity:  "critical",
		},
	})

	plugin.bufferMutex.RLock()
	assert.Len(t, plugin.complianceBuffer, 3)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_WithComplianceEvents(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{
			IncludeProxySummaries: true,
			CompressionEnabled:   false,
		},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	now := time.Now()
	complianceData := []ComplianceEventBuffer{
		{
			AppID:       1,
			UserID:      10,
			LLMID:       5,
			FilterName:  "pii-filter",
			FilterScope: "proxy_request",
			EventType:   "pii_redacted",
			Severity:    "warning",
			Description: "SSN pattern found",
			Metadata:    `{"pattern":"ssn","count":2}`,
			Vendor:      "openai",
			ModelName:   "gpt-4",
			Timestamp:   now,
		},
		{
			AppID:       2,
			UserID:      20,
			LLMID:       8,
			FilterName:  "tone-filter",
			FilterScope: "proxy_response",
			EventType:   "content_rewritten",
			Severity:    "info",
			Description: "Tone adjusted",
			Vendor:      "anthropic",
			ModelName:   "claude-3",
			Timestamp:   now.Add(1 * time.Second),
		},
	}

	pulse := plugin.buildPulseMessage(nil, nil, nil, nil, complianceData, 42)

	require.NotNil(t, pulse)
	assert.Equal(t, "test-edge", pulse.EdgeId)
	assert.Equal(t, "test", pulse.EdgeNamespace)
	assert.Equal(t, uint64(42), pulse.SequenceNumber)
	assert.Equal(t, uint32(2), pulse.TotalRecords)

	// Verify compliance events in proto message
	require.Len(t, pulse.ComplianceEvents, 2)

	ce1 := pulse.ComplianceEvents[0]
	assert.Equal(t, uint32(1), ce1.AppId)
	assert.Equal(t, uint32(10), ce1.UserId)
	assert.Equal(t, uint32(5), ce1.LlmId)
	assert.Equal(t, "pii-filter", ce1.FilterName)
	assert.Equal(t, "proxy_request", ce1.FilterScope)
	assert.Equal(t, "pii_redacted", ce1.EventType)
	assert.Equal(t, "warning", ce1.Severity)
	assert.Equal(t, "SSN pattern found", ce1.Description)
	assert.Equal(t, `{"pattern":"ssn","count":2}`, ce1.Metadata)
	assert.Equal(t, "openai", ce1.Vendor)
	assert.Equal(t, "gpt-4", ce1.ModelName)
	assert.NotNil(t, ce1.Timestamp)

	ce2 := pulse.ComplianceEvents[1]
	assert.Equal(t, uint32(2), ce2.AppId)
	assert.Equal(t, "content_rewritten", ce2.EventType)
	assert.Equal(t, "info", ce2.Severity)
	assert.Equal(t, "anthropic", ce2.Vendor)

	// Verify other collections are empty
	assert.Empty(t, pulse.AnalyticsEvents)
	assert.Empty(t, pulse.BudgetEvents)
	assert.Empty(t, pulse.ProxySummaries)
}

func TestAnalyticsPulsePlugin_BufferComplianceEvents_EmptySlice(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{},
	}

	// Should not panic on empty slice
	plugin.BufferComplianceEvents(nil)
	plugin.BufferComplianceEvents([]ComplianceEventBuffer{})

	plugin.bufferMutex.RLock()
	assert.Len(t, plugin.complianceBuffer, 0)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BufferComplianceEvents_ConcurrentAccess(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{},
	}

	// Simulate concurrent buffering from multiple goroutines
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			plugin.BufferComplianceEvents([]ComplianceEventBuffer{
				{
					AppID:     uint32(id),
					EventType: "concurrent_test",
					Severity:  "info",
					Timestamp: time.Now(),
				},
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	plugin.bufferMutex.RLock()
	assert.Len(t, plugin.complianceBuffer, 10)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_EmptyComplianceData(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{
			IncludeProxySummaries: false,
			CompressionEnabled:   false,
		},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	// Empty compliance data should produce nil ComplianceEvents (not empty slice)
	pulse := plugin.buildPulseMessage(nil, nil, nil, nil, nil, 1)
	require.NotNil(t, pulse)
	assert.Nil(t, pulse.ComplianceEvents)
	assert.Equal(t, uint32(0), pulse.TotalRecords)

	// Empty slice should also produce nil
	pulse = plugin.buildPulseMessage(nil, nil, nil, nil, []ComplianceEventBuffer{}, 2)
	require.NotNil(t, pulse)
	assert.Nil(t, pulse.ComplianceEvents)
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_ComplianceWithMissingFields(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{
			IncludeProxySummaries: false,
			CompressionEnabled:   false,
		},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	// Events with minimal/missing fields should still serialize without error
	complianceData := []ComplianceEventBuffer{
		{
			// Only required fields
			EventType: "minimal_event",
			Severity:  "info",
			Timestamp: time.Now(),
		},
		{
			// Zero-value IDs, empty strings
			AppID:       0,
			UserID:      0,
			LLMID:       0,
			FilterName:  "",
			FilterScope: "",
			EventType:   "empty_fields",
			Severity:    "warning",
			Description: "",
			Metadata:    "",
			Vendor:      "",
			ModelName:   "",
			Timestamp:   time.Time{}, // zero time
		},
	}

	pulse := plugin.buildPulseMessage(nil, nil, nil, nil, complianceData, 1)
	require.NotNil(t, pulse)
	require.Len(t, pulse.ComplianceEvents, 2)

	assert.Equal(t, "minimal_event", pulse.ComplianceEvents[0].EventType)
	assert.Equal(t, uint32(0), pulse.ComplianceEvents[0].AppId) // zero value preserved

	assert.Equal(t, "empty_fields", pulse.ComplianceEvents[1].EventType)
	assert.Equal(t, "", pulse.ComplianceEvents[1].FilterName) // empty string preserved
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_MixedData(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{
			IncludeProxySummaries: false,
			CompressionEnabled:   false,
		},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	complianceData := []ComplianceEventBuffer{
		{
			AppID:     1,
			EventType: "pii_redacted",
			Severity:  "warning",
			Timestamp: time.Now(),
		},
	}

	budgetData := []BudgetUsageBuffer{
		{
			AppID:     1,
			Cost:      0.005,
			Timestamp: time.Now(),
		},
	}

	pulse := plugin.buildPulseMessage(nil, nil, budgetData, nil, complianceData, 1)

	require.NotNil(t, pulse)
	assert.Equal(t, uint32(2), pulse.TotalRecords) // 1 budget + 1 compliance
	assert.Len(t, pulse.ComplianceEvents, 1)
	assert.Len(t, pulse.BudgetEvents, 1)
	assert.Empty(t, pulse.AnalyticsEvents)
}
