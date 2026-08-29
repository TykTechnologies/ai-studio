package plugins

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsPulsePlugin_BufferToolCalls(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{config: &PulsePluginConfig{}}

	plugin.BufferToolCalls([]ToolCallBuffer{
		{ToolID: 4, OperationID: "getExchangeRate", ExecTimeMs: 120, Timestamp: time.Now()},
		{ToolID: 4, OperationID: "getExchangeRates", ExecTimeMs: 95, Timestamp: time.Now()},
	})

	plugin.bufferMutex.RLock()
	require.Len(t, plugin.toolCallBuffer, 2)
	assert.Equal(t, "getExchangeRate", plugin.toolCallBuffer[0].OperationID)
	assert.Equal(t, "getExchangeRates", plugin.toolCallBuffer[1].OperationID)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BufferToolCalls_EmptySlice(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{config: &PulsePluginConfig{}}

	plugin.BufferToolCalls(nil)
	plugin.BufferToolCalls([]ToolCallBuffer{})

	plugin.bufferMutex.RLock()
	assert.Empty(t, plugin.toolCallBuffer)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BufferToolCalls_ConcurrentAccess(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{config: &PulsePluginConfig{}}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			plugin.BufferToolCalls([]ToolCallBuffer{
				{ToolID: uint32(id), OperationID: "concurrent", Timestamp: time.Now()},
			})
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	plugin.bufferMutex.RLock()
	assert.Len(t, plugin.toolCallBuffer, 10)
	plugin.bufferMutex.RUnlock()
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_WithToolCalls(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	now := time.Now()
	toolCalls := []ToolCallBuffer{
		{ToolID: 4, OperationID: "getExchangeRate", ExecTimeMs: 120, Timestamp: now},
		{ToolID: 4, OperationID: "getExchangeRates", ExecTimeMs: 95, Timestamp: now.Add(time.Second)},
	}

	pulse := plugin.buildPulseMessage(nil, nil, nil, nil, nil, toolCalls, 7)

	require.NotNil(t, pulse)
	require.Len(t, pulse.ToolCalls, 2)

	// Each call has to carry the operation it was, or the control plane has
	// nothing to attribute it to.
	assert.Equal(t, uint32(4), pulse.ToolCalls[0].ToolId)
	assert.Equal(t, "getExchangeRate", pulse.ToolCalls[0].OperationId)
	assert.Equal(t, int32(120), pulse.ToolCalls[0].ExecTimeMs)
	require.NotNil(t, pulse.ToolCalls[0].Timestamp)
	assert.WithinDuration(t, now, pulse.ToolCalls[0].Timestamp.AsTime(), time.Second)

	assert.Equal(t, "getExchangeRates", pulse.ToolCalls[1].OperationId)

	// Tool calls count toward the pulse's record total.
	assert.Equal(t, uint32(2), pulse.TotalRecords)
}

func TestAnalyticsPulsePlugin_BuildPulseMessage_NoToolCalls(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-5 * time.Minute),
	}

	pulse := plugin.buildPulseMessage(nil, nil, nil, nil, nil, nil, 1)
	require.NotNil(t, pulse)
	assert.Nil(t, pulse.ToolCalls)
	assert.Equal(t, uint32(0), pulse.TotalRecords)
}

func TestAnalyticsPulsePlugin_GetStatsIncludesToolCalls(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{config: &PulsePluginConfig{IntervalSeconds: 10}}
	plugin.BufferToolCalls([]ToolCallBuffer{{ToolID: 1, OperationID: "op", Timestamp: time.Now()}})

	stats := plugin.GetStats()
	assert.Equal(t, 1, stats["tool_calls_buffered"])
	assert.Equal(t, 1, stats["current_buffer_size"])
}
