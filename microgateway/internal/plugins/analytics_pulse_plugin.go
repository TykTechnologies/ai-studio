// internal/plugins/analytics_pulse_plugin.go
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AnalyticsPulsePlugin is a built-in data collection plugin that sends analytics data to control server
type AnalyticsPulsePlugin struct {
	config         *PulsePluginConfig
	edgeID         string
	edgeNamespace  string
	grpcClient     pb.ConfigurationSyncServiceClient

	// Buffered data
	analyticsBuffer    []database.AnalyticsEvent
	analyticsMetadata  []AnalyticsMetadata // Store additional data not in database.AnalyticsEvent
	budgetBuffer       []BudgetUsageBuffer
	proxyBuffer        []ProxyLogBuffer
	bufferMutex        sync.RWMutex

	// Pulse management
	pulseTimer      *time.Timer
	sequenceNumber  uint64
	lastPulseTime   time.Time
	ctx             context.Context
	cancel          context.CancelFunc

	// Statistics
	totalRecordsSent uint64
	totalPulsesSent  uint64
	lastError        error
	lastErrorTime    time.Time
}

// PulsePluginConfig holds configuration for the analytics pulse plugin
type PulsePluginConfig struct {
	IntervalSeconds             int      `json:"interval_seconds"`
	MaxBatchSize                int      `json:"max_batch_size"`
	MaxBufferSize               int      `json:"max_buffer_size"`
	CompressionEnabled          bool     `json:"compression_enabled"`
	IncludeProxySummaries       bool     `json:"include_proxy_summaries"`
	IncludeRequestResponseData  bool     `json:"include_request_response_data"`
	EdgeRetentionHours          int      `json:"edge_retention_hours"`
	ExcludedVendors             []string `json:"excluded_vendors"`
	TimeoutSeconds              int      `json:"timeout_seconds"`
	MaxRetries                  int      `json:"max_retries"`
	RetryIntervalSecs           int      `json:"retry_interval_secs"`
}

// AnalyticsMetadata stores additional analytics data not in database.AnalyticsEvent
type AnalyticsMetadata struct {
	RequestID    string
	ModelName    string
	Vendor       string
	RequestBody  string
	ResponseBody string
}

// Buffer structures for batching data
type BudgetUsageBuffer struct {
	AppID            uint32
	LLMID            uint32
	TokensUsed       int64
	Cost             float64
	PromptTokens     int64
	CompletionTokens int64
	RequestsCount    uint32
	Timestamp        time.Time
	PeriodStart      time.Time
	PeriodEnd        time.Time
}

type ProxyLogBuffer struct {
	AppID              uint32
	UserID             uint32
	Vendor             string
	ResponseCode       int32
	RequestCount       uint32
	TotalRequestBytes  uint64
	TotalResponseBytes uint64
	AvgLatencyMs       uint32
	ErrorCount         uint32
	FirstRequest       time.Time
	LastRequest        time.Time
	UniqueModels       []string
	TotalTokens        uint32
	TotalCost          float64
}

// NewAnalyticsPulsePlugin creates a new built-in analytics pulse plugin
func NewAnalyticsPulsePlugin(
	edgeID, edgeNamespace string,
	grpcClient pb.ConfigurationSyncServiceClient,
	config map[string]interface{},
) (*AnalyticsPulsePlugin, error) {
	// Parse configuration
	pluginConfig, err := parsePluginConfig(config)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	plugin := &AnalyticsPulsePlugin{
		config:         pluginConfig,
		edgeID:         edgeID,
		edgeNamespace:  edgeNamespace,
		grpcClient:     grpcClient,
		ctx:            ctx,
		cancel:         cancel,
		sequenceNumber: 1,
		lastPulseTime:  time.Now(),
	}

	log.Info().
		Str("edge_id", edgeID).
		Int("interval_seconds", pluginConfig.IntervalSeconds).
		Int("max_batch_size", pluginConfig.MaxBatchSize).
		Msg("Analytics pulse plugin created")

	return plugin, nil
}

// parsePluginConfig parses the plugin configuration from the config map
func parsePluginConfig(config map[string]interface{}) (*PulsePluginConfig, error) {
	// Convert config map to JSON and back to struct for easy parsing
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var pluginConfig PulsePluginConfig
	if err := json.Unmarshal(configJSON, &pluginConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set defaults for missing values
	if pluginConfig.IntervalSeconds == 0 {
		pluginConfig.IntervalSeconds = 300 // 5 minutes
	}
	if pluginConfig.MaxBatchSize == 0 {
		pluginConfig.MaxBatchSize = 1000
	}
	if pluginConfig.MaxBufferSize == 0 {
		pluginConfig.MaxBufferSize = 10000
	}
	if pluginConfig.TimeoutSeconds == 0 {
		pluginConfig.TimeoutSeconds = 30
	}
	if pluginConfig.MaxRetries == 0 {
		pluginConfig.MaxRetries = 3
	}
	if pluginConfig.RetryIntervalSecs == 0 {
		pluginConfig.RetryIntervalSecs = 5
	}
	if pluginConfig.EdgeRetentionHours == 0 {
		pluginConfig.EdgeRetentionHours = 24
	}

	return &pluginConfig, nil
}

// GetHookType returns the hook type this plugin implements
func (p *AnalyticsPulsePlugin) GetHookType() interfaces.HookType {
	return interfaces.HookTypeDataCollection
}

// GetName returns the plugin name
func (p *AnalyticsPulsePlugin) GetName() string {
	return "analytics_pulse"
}

// GetVersion returns the plugin version
func (p *AnalyticsPulsePlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown performs cleanup when plugin is unloaded
func (p *AnalyticsPulsePlugin) Shutdown() error {
	return p.Stop()
}

// Initialize initializes the plugin
func (p *AnalyticsPulsePlugin) Initialize(config map[string]interface{}) error {
	log.Info().
		Str("plugin", "analytics_pulse").
		Msg("Initializing built-in analytics pulse plugin")

	// Start the pulse timer
	p.schedulePulse()

	log.Info().
		Int("interval_seconds", p.config.IntervalSeconds).
		Msg("Analytics pulse plugin initialized successfully")

	return nil
}

// HandleProxyLog processes proxy log data for analytics pulse
func (p *AnalyticsPulsePlugin) HandleProxyLog(ctx context.Context, req *interfaces.ProxyLogData, pluginCtx *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	if p.config.IncludeProxySummaries {
		p.bufferMutex.Lock()
		defer p.bufferMutex.Unlock()

		// Create proxy log summary
		summary := ProxyLogBuffer{
			AppID:              uint32(req.AppID),
			UserID:             uint32(req.UserID),
			Vendor:             req.Vendor,
			ResponseCode:       int32(req.ResponseCode),
			RequestCount:       1,
			TotalRequestBytes:  uint64(len(req.RequestBody)),
			TotalResponseBytes: uint64(len(req.ResponseBody)),
			FirstRequest:       req.Timestamp,
			LastRequest:        req.Timestamp,
		}

		p.proxyBuffer = append(p.proxyBuffer, summary)

		log.Debug().
			Str("request_id", req.RequestID).
			Msg("Proxy log summary buffered for pulse")
	}

	return &interfaces.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// HandleAnalytics processes analytics data for pulse transmission
func (p *AnalyticsPulsePlugin) HandleAnalytics(ctx context.Context, req *interfaces.AnalyticsData, pluginCtx *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	// Check if vendor should be excluded
	if p.isVendorExcluded(req.Vendor) {
		log.Debug().
			Str("vendor", req.Vendor).
			Msg("Vendor excluded from analytics pulse")
		return &interfaces.DataCollectionResponse{
			Success: true,
			Handled: false, // Don't handle excluded vendors
		}, nil
	}

	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()

	// Convert to database analytics event format for buffering
	event := database.AnalyticsEvent{
		RequestID:              req.RequestID,
		AppID:                  req.AppID,
		LLMID:                  &req.LLMID,

		// Fields matching LLMChatRecord for parity
		UserID:                 req.UserID,
		Name:                   req.ModelName,
		Vendor:                 req.Vendor,
		InteractionType:        "proxy",
		Choices:                int(req.Choices),
		ToolCalls:              int(req.ToolCalls),
		ChatID:                 "",
		Currency:               req.Currency,

		// Request/Response details
		Endpoint:               fmt.Sprintf("/%s", req.Vendor),
		Method:                 "POST",
		StatusCode:             200, // Default success

		// Token tracking (using new field names)
		PromptTokens:           req.PromptTokens,
		ResponseTokens:         req.ResponseTokens,
		TotalTokens:            req.TotalTokens,
		CacheWritePromptTokens: req.CacheWritePromptTokens,
		CacheReadPromptTokens:  req.CacheReadPromptTokens,

		// Cost and timing
		Cost:                   req.Cost,
		TotalTimeMS:            0, // Not available

		ErrorMessage:           "",
		TimeStamp:              req.Timestamp,
		CreatedAt:              req.Timestamp,
	}

	// Store metadata for pulse transmission
	metadata := AnalyticsMetadata{
		RequestID:    req.RequestID,
		ModelName:    req.ModelName,
		Vendor:       req.Vendor,
		RequestBody:  req.RequestBody,  // From analytics interface
		ResponseBody: req.ResponseBody, // From analytics interface
	}

	p.analyticsBuffer = append(p.analyticsBuffer, event)
	p.analyticsMetadata = append(p.analyticsMetadata, metadata)

	// Check if buffer is getting full
	totalBuffered := len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer)
	if totalBuffered >= p.config.MaxBufferSize {
		log.Warn().
			Int("total_buffered", totalBuffered).
			Int("max_buffer_size", p.config.MaxBufferSize).
			Msg("Analytics buffer is full, triggering immediate pulse")
		go p.sendPulseNow()
	}

	log.Debug().
		Str("request_id", req.RequestID).
		Int("total_buffered", totalBuffered).
		Msg("Analytics event buffered for pulse")

	return &interfaces.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// HandleBudgetUsage processes budget usage data for pulse transmission
func (p *AnalyticsPulsePlugin) HandleBudgetUsage(ctx context.Context, req *interfaces.BudgetUsageData, pluginCtx *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()

	budgetData := BudgetUsageBuffer{
		AppID:            uint32(req.AppID),
		LLMID:            uint32(req.LLMID),
		TokensUsed:       req.TokensUsed,
		Cost:             req.Cost,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		RequestsCount:    uint32(req.RequestsCount),
		Timestamp:        req.Timestamp,
		PeriodStart:      req.PeriodStart,
		PeriodEnd:        req.PeriodEnd,
	}

	p.budgetBuffer = append(p.budgetBuffer, budgetData)

	log.Debug().
		Str("request_id", req.RequestID).
		Uint("app_id", req.AppID).
		Float64("cost", req.Cost).
		Msg("Budget usage buffered for pulse")

	return &interfaces.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// schedulePulse schedules the next pulse
func (p *AnalyticsPulsePlugin) schedulePulse() {
	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}

	interval := time.Duration(p.config.IntervalSeconds) * time.Second
	p.pulseTimer = time.AfterFunc(interval, func() {
		p.sendPulse()
		p.schedulePulse() // Schedule the next pulse
	})

	log.Debug().
		Dur("next_pulse_in", interval).
		Msg("Next analytics pulse scheduled")
}

// sendPulseNow sends a pulse immediately
func (p *AnalyticsPulsePlugin) sendPulseNow() {
	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}
	p.sendPulse()
	p.schedulePulse()
}

// sendPulse creates and sends an analytics pulse to control server
func (p *AnalyticsPulsePlugin) sendPulse() {
	p.bufferMutex.Lock()

	// Check if there's any data to send
	if len(p.analyticsBuffer) == 0 && len(p.budgetBuffer) == 0 && len(p.proxyBuffer) == 0 {
		p.bufferMutex.Unlock()
		log.Debug().Msg("No analytics data to pulse - skipping")
		return
	}

	// Create snapshots and clear buffers
	analyticsSnapshot := make([]database.AnalyticsEvent, len(p.analyticsBuffer))
	copy(analyticsSnapshot, p.analyticsBuffer)
	p.analyticsBuffer = p.analyticsBuffer[:0]

	metadataSnapshot := make([]AnalyticsMetadata, len(p.analyticsMetadata))
	copy(metadataSnapshot, p.analyticsMetadata)
	p.analyticsMetadata = p.analyticsMetadata[:0]

	budgetSnapshot := make([]BudgetUsageBuffer, len(p.budgetBuffer))
	copy(budgetSnapshot, p.budgetBuffer)
	p.budgetBuffer = p.budgetBuffer[:0]

	proxySnapshot := make([]ProxyLogBuffer, len(p.proxyBuffer))
	copy(proxySnapshot, p.proxyBuffer)
	p.proxyBuffer = p.proxyBuffer[:0]

	sequenceNum := p.sequenceNumber
	p.sequenceNumber++

	p.bufferMutex.Unlock()

	// Build and send pulse
	pulse := p.buildPulseMessage(analyticsSnapshot, metadataSnapshot, budgetSnapshot, proxySnapshot, sequenceNum)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.config.TimeoutSeconds)*time.Second)
	defer cancel()

	log.Info().
		Uint64("sequence", sequenceNum).
		Int("analytics_events", len(analyticsSnapshot)).
		Int("budget_events", len(budgetSnapshot)).
		Int("proxy_summaries", len(proxySnapshot)).
		Uint32("total_records", pulse.TotalRecords).
		Msg("Sending analytics pulse to control server")

	resp, err := p.grpcClient.SendAnalyticsPulse(ctx, pulse)
	if err != nil {
		p.lastError = err
		p.lastErrorTime = time.Now()
		log.Error().
			Err(err).
			Uint64("sequence", sequenceNum).
			Msg("Failed to send analytics pulse")
		return
	}

	p.totalPulsesSent++
	p.totalRecordsSent += uint64(pulse.TotalRecords)
	p.lastPulseTime = time.Now()

	if resp.Success {
		log.Info().
			Uint64("sequence", sequenceNum).
			Uint64("processed_records", resp.ProcessedRecords).
			Msg("Analytics pulse sent successfully")
	} else {
		log.Error().
			Str("message", resp.Message).
			Uint64("sequence", sequenceNum).
			Msg("Analytics pulse rejected by control server")
	}
}

// buildPulseMessage constructs the analytics pulse protobuf message
func (p *AnalyticsPulsePlugin) buildPulseMessage(
	analyticsData []database.AnalyticsEvent,
	metadata []AnalyticsMetadata,
	budgetData []BudgetUsageBuffer,
	proxyData []ProxyLogBuffer,
	sequenceNum uint64,
) *pb.AnalyticsPulse {
	now := time.Now()

	// Convert analytics events
	var analyticsEvents []*pb.AnalyticsEvent
	for i, event := range analyticsData {
		llmID := uint32(0)
		if event.LLMID != nil {
			llmID = uint32(*event.LLMID)
		}

		// Get metadata for this event
		var eventMetadata AnalyticsMetadata
		if i < len(metadata) {
			eventMetadata = metadata[i]
		}

		// Include request/response data if configured
		requestBody := ""
		responseBody := ""
		if p.config.IncludeRequestResponseData {
			requestBody = eventMetadata.RequestBody
			responseBody = eventMetadata.ResponseBody
		}

		analyticsEvents = append(analyticsEvents, &pb.AnalyticsEvent{
			RequestId:               event.RequestID,
			AppId:                   uint32(event.AppID),
			LlmId:                   llmID,
			UserId:                  uint32(event.UserID),
			Endpoint:                event.Endpoint,
			Method:                  event.Method,
			StatusCode:              int32(event.StatusCode),

			// Fields matching LLMChatRecord for parity
			ModelName:               event.Name,
			Vendor:                  event.Vendor,
			InteractionType:         event.InteractionType,
			Choices:                 uint32(event.Choices),
			ToolCalls:               uint32(event.ToolCalls),
			ChatId:                  event.ChatID,
			Currency:                event.Currency,

			// Token tracking (using field names from new schema)
			RequestTokens:           uint32(event.PromptTokens),
			ResponseTokens:          uint32(event.ResponseTokens),
			TotalTokens:             uint32(event.TotalTokens),
			CacheWritePromptTokens:  uint32(event.CacheWritePromptTokens),
			CacheReadPromptTokens:   uint32(event.CacheReadPromptTokens),

			// Cost and timing
			Cost:                    event.Cost,
			LatencyMs:               uint32(event.TotalTimeMS),

			Timestamp:               timestamppb.New(event.TimeStamp),
			ErrorMessage:            event.ErrorMessage,
			RequestSizeBytes:        uint32(len(event.RequestBody)),
			ResponseSizeBytes:       uint32(len(event.ResponseBody)),
			RequestBody:             requestBody,
			ResponseBody:            responseBody,
		})
	}

	// Convert budget events
	var budgetEvents []*pb.BudgetUsageEvent
	for _, budget := range budgetData {
		budgetEvents = append(budgetEvents, &pb.BudgetUsageEvent{
			AppId:            budget.AppID,
			LlmId:            budget.LLMID,
			TokensUsed:       budget.TokensUsed,
			Cost:             budget.Cost,
			PromptTokens:     budget.PromptTokens,
			CompletionTokens: budget.CompletionTokens,
			RequestsCount:    budget.RequestsCount,
			Timestamp:        timestamppb.New(budget.Timestamp),
			PeriodStart:      timestamppb.New(budget.PeriodStart),
			PeriodEnd:        timestamppb.New(budget.PeriodEnd),
		})
	}

	// Convert proxy summaries
	var proxySummaries []*pb.ProxyLogSummary
	if p.config.IncludeProxySummaries {
		for _, proxy := range proxyData {
			proxySummaries = append(proxySummaries, &pb.ProxyLogSummary{
				AppId:               proxy.AppID,
				UserId:              proxy.UserID,
				Vendor:              proxy.Vendor,
				ResponseCode:        proxy.ResponseCode,
				RequestCount:        proxy.RequestCount,
				TotalRequestBytes:   proxy.TotalRequestBytes,
				TotalResponseBytes:  proxy.TotalResponseBytes,
				AvgLatencyMs:        proxy.AvgLatencyMs,
				ErrorCount:          proxy.ErrorCount,
				FirstRequest:        timestamppb.New(proxy.FirstRequest),
				LastRequest:         timestamppb.New(proxy.LastRequest),
				UniqueModels:        proxy.UniqueModels,
				TotalTokens:         proxy.TotalTokens,
				TotalCost:           proxy.TotalCost,
			})
		}
	}

	totalRecords := uint32(len(analyticsEvents) + len(budgetEvents) + len(proxySummaries))

	return &pb.AnalyticsPulse{
		EdgeId:           p.edgeID,
		EdgeNamespace:    p.edgeNamespace,
		PulseTimestamp:   timestamppb.New(now),
		DataFrom:         timestamppb.New(p.lastPulseTime),
		DataTo:           timestamppb.New(now),
		SequenceNumber:   sequenceNum,
		AnalyticsEvents:  analyticsEvents,
		BudgetEvents:     budgetEvents,
		ProxySummaries:   proxySummaries,
		IsCompressed:     p.config.CompressionEnabled,
		TotalRecords:     totalRecords,
		DataSizeBytes:    0, // TODO: Calculate if needed
	}
}

// isVendorExcluded checks if a vendor should be excluded from pulses
func (p *AnalyticsPulsePlugin) isVendorExcluded(vendor string) bool {
	for _, excluded := range p.config.ExcludedVendors {
		if excluded != "" && strings.Contains(strings.ToLower(vendor), strings.ToLower(excluded)) {
			return true
		}
	}
	return false
}

// Stop stops the analytics pulse plugin
func (p *AnalyticsPulsePlugin) Stop() error {
	log.Info().Str("plugin", "analytics_pulse").Msg("Stopping analytics pulse plugin")

	if p.cancel != nil {
		p.cancel()
	}

	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}

	// Send any remaining buffered data
	p.sendPulseNow()

	log.Info().
		Uint64("total_pulses_sent", p.totalPulsesSent).
		Uint64("total_records_sent", p.totalRecordsSent).
		Msg("Analytics pulse plugin stopped")

	return nil
}

// GetStats returns current plugin statistics
func (p *AnalyticsPulsePlugin) GetStats() map[string]interface{} {
	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()

	stats := map[string]interface{}{
		"total_pulses_sent":     p.totalPulsesSent,
		"total_records_sent":    p.totalRecordsSent,
		"current_buffer_size":   len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer),
		"analytics_buffered":    len(p.analyticsBuffer),
		"budget_buffered":       len(p.budgetBuffer),
		"proxy_buffered":        len(p.proxyBuffer),
		"sequence_number":       p.sequenceNumber,
		"last_pulse_time":       p.lastPulseTime,
		"pulse_interval_seconds": p.config.IntervalSeconds,
	}

	if p.lastError != nil {
		stats["last_error"] = p.lastError.Error()
		stats["last_error_time"] = p.lastErrorTime
	}

	return stats
}