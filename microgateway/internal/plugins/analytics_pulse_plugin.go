// internal/plugins/analytics_pulse_plugin.go
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	complianceBuffer   []ComplianceEventBuffer
	toolCallBuffer     []ToolCallBuffer
	bufferMutex        sync.RWMutex

	// Pulse management
	pulseTimer      *time.Timer
	sequenceNumber  uint64
	lastPulseTime   time.Time
	ctx             context.Context
	cancel          context.CancelFunc
	// sending is set while a pulse (and its retry ladder) is in flight so the
	// timer and a buffer-full trigger never run sendPulse concurrently.
	sending atomic.Bool
	// retryInterval is RetryIntervalSecs as a duration; tests set it directly
	// so a retry ladder can be exercised in milliseconds.
	retryInterval time.Duration

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

// ComplianceEventBuffer holds a compliance event for batching in pulses
type ComplianceEventBuffer struct {
	AppID       uint32
	UserID      uint32
	LLMID       uint32
	FilterName  string
	FilterScope string
	EventType   string
	Severity    string
	Description string
	Metadata    string // JSON blob
	Vendor      string
	ModelName   string
	Timestamp   time.Time
}

// ToolCallBuffer holds a single tool operation call for batching in pulses.
// The aggregate ToolCalls counter on an analytics event says how many calls a
// request made but not which operations they were, so tool analytics on the
// control plane has nothing to attribute edge traffic to without these.
type ToolCallBuffer struct {
	ToolID      uint32
	OperationID string
	ExecTimeMs  int32
	Timestamp   time.Time
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
		retryInterval:  time.Duration(pluginConfig.RetryIntervalSecs) * time.Second,
	}

	log.Debug().
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
	log.Debug().
		Str("plugin", "analytics_pulse").
		Msg("Initializing built-in analytics pulse plugin")

	// Start the pulse timer
	p.schedulePulse()

	log.Debug().
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

	// Use actual status code, default to 200 for backwards compatibility
	statusCode := req.StatusCode
	if statusCode == 0 {
		statusCode = 200
	}

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
		StatusCode:             statusCode, // Use actual HTTP status code (e.g., 403 for budget exceeded)

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

		// Failover marker (nil / 0 for a primary attempt)
		FailoverFromLLMID:      req.FailoverFromLLMID,
		FailoverAttempt:        req.FailoverAttempt,
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

	// Check if buffer is getting full. While the last pulse is still inside
	// its backoff a full buffer must not fire pulse after pulse at a hub that
	// has just failed; the timer picks the data up once the interval passes.
	totalBuffered := p.totalBufferedLocked()
	if totalBuffered >= p.config.MaxBufferSize {
		if p.inBackoffLocked(time.Now()) {
			log.Debug().
				Int("total_buffered", totalBuffered).
				Msg("Analytics buffer is full but the last pulse failed recently - waiting for backoff")
		} else {
			log.Warn().
				Int("total_buffered", totalBuffered).
				Int("max_buffer_size", p.config.MaxBufferSize).
				Msg("Analytics buffer is full, triggering immediate pulse")
			go p.sendPulseNow()
		}
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

// BufferComplianceEvents adds compliance events to the buffer for pulse transmission.
// Called by the microgateway analytics handler when filter scripts report compliance events.
func (p *AnalyticsPulsePlugin) BufferComplianceEvents(events []ComplianceEventBuffer) {
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()

	p.complianceBuffer = append(p.complianceBuffer, events...)

	log.Debug().
		Int("count", len(events)).
		Int("buffer_size", len(p.complianceBuffer)).
		Msg("Compliance events buffered for pulse")
}

// BufferToolCalls adds tool operation calls to the buffer for pulse
// transmission. Called by the microgateway analytics handler whenever a tool
// operation is served by this edge, over REST or over MCP.
func (p *AnalyticsPulsePlugin) BufferToolCalls(calls []ToolCallBuffer) {
	if len(calls) == 0 {
		return
	}

	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()

	p.toolCallBuffer = append(p.toolCallBuffer, calls...)

	log.Debug().
		Int("count", len(calls)).
		Int("buffer_size", len(p.toolCallBuffer)).
		Msg("Tool calls buffered for pulse")
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

// pulseSnapshot is one pulse's worth of buffered data, taken out of the
// buffers under the lock so it can be sent without holding them. If the send
// fails it is put back in front of whatever arrived in the meantime.
type pulseSnapshot struct {
	analytics  []database.AnalyticsEvent
	metadata   []AnalyticsMetadata
	budget     []BudgetUsageBuffer
	proxy      []ProxyLogBuffer
	compliance []ComplianceEventBuffer
	toolCalls  []ToolCallBuffer
}

func (s *pulseSnapshot) empty() bool {
	return len(s.analytics) == 0 && len(s.budget) == 0 && len(s.proxy) == 0 &&
		len(s.compliance) == 0 && len(s.toolCalls) == 0
}

// totalBufferedLocked counts every record waiting for a pulse. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) totalBufferedLocked() int {
	return len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer) +
		len(p.complianceBuffer) + len(p.toolCallBuffer)
}

// retryWait is the pause between attempts of one pulse, and the backoff after
// a pulse has failed for good.
func (p *AnalyticsPulsePlugin) retryWait() time.Duration {
	if p.retryInterval > 0 {
		return p.retryInterval
	}
	return time.Duration(p.config.RetryIntervalSecs) * time.Second
}

// inBackoffLocked reports whether the last pulse failed less than one retry
// interval ago. Caller holds bufferMutex (lastError is written under it).
func (p *AnalyticsPulsePlugin) inBackoffLocked(now time.Time) bool {
	return p.lastError != nil && now.Sub(p.lastErrorTime) < p.retryWait()
}

// stopping reports whether the plugin has been told to shut down.
func (p *AnalyticsPulsePlugin) stopping() bool {
	return p.ctx != nil && p.ctx.Err() != nil
}

// takeSnapshotLocked moves everything buffered into a snapshot. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) takeSnapshotLocked() pulseSnapshot {
	snap := pulseSnapshot{
		analytics:  append([]database.AnalyticsEvent(nil), p.analyticsBuffer...),
		metadata:   append([]AnalyticsMetadata(nil), p.analyticsMetadata...),
		budget:     append([]BudgetUsageBuffer(nil), p.budgetBuffer...),
		proxy:      append([]ProxyLogBuffer(nil), p.proxyBuffer...),
		compliance: append([]ComplianceEventBuffer(nil), p.complianceBuffer...),
		toolCalls:  append([]ToolCallBuffer(nil), p.toolCallBuffer...),
	}
	p.analyticsBuffer = p.analyticsBuffer[:0]
	p.analyticsMetadata = p.analyticsMetadata[:0]
	p.budgetBuffer = p.budgetBuffer[:0]
	p.proxyBuffer = p.proxyBuffer[:0]
	p.complianceBuffer = p.complianceBuffer[:0]
	p.toolCallBuffer = p.toolCallBuffer[:0]
	return snap
}

// restoreSnapshotLocked puts an unsent snapshot back at the front of the
// buffers (it is older than anything buffered since), drops records that have
// aged past EdgeRetentionHours, and trims the buffers to MaxBufferSize by
// discarding the oldest records. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) restoreSnapshotLocked(snap pulseSnapshot, now time.Time) {
	cutoff := now.Add(-time.Duration(p.config.EdgeRetentionHours) * time.Hour)
	fresh := func(ts time.Time) bool { return p.config.EdgeRetentionHours <= 0 || !ts.Before(cutoff) }

	// Analytics events and their metadata are paired by index; keep them in step.
	analytics := make([]database.AnalyticsEvent, 0, len(snap.analytics))
	metadata := make([]AnalyticsMetadata, 0, len(snap.analytics))
	for i, ev := range snap.analytics {
		if !fresh(ev.TimeStamp) {
			continue
		}
		analytics = append(analytics, ev)
		if i < len(snap.metadata) {
			metadata = append(metadata, snap.metadata[i])
		} else {
			metadata = append(metadata, AnalyticsMetadata{})
		}
	}
	budget := snap.budget[:0:0]
	for _, b := range snap.budget {
		if fresh(b.Timestamp) {
			budget = append(budget, b)
		}
	}
	proxy := snap.proxy[:0:0]
	for _, pr := range snap.proxy {
		if fresh(pr.LastRequest) {
			proxy = append(proxy, pr)
		}
	}
	compliance := snap.compliance[:0:0]
	for _, ce := range snap.compliance {
		if fresh(ce.Timestamp) {
			compliance = append(compliance, ce)
		}
	}
	toolCalls := snap.toolCalls[:0:0]
	for _, tc := range snap.toolCalls {
		if fresh(tc.Timestamp) {
			toolCalls = append(toolCalls, tc)
		}
	}

	expired := (len(snap.analytics) - len(analytics)) + (len(snap.budget) - len(budget)) +
		(len(snap.proxy) - len(proxy)) + (len(snap.compliance) - len(compliance)) +
		(len(snap.toolCalls) - len(toolCalls))

	p.analyticsBuffer = append(analytics, p.analyticsBuffer...)
	p.analyticsMetadata = append(metadata, p.analyticsMetadata...)
	p.budgetBuffer = append(budget, p.budgetBuffer...)
	p.proxyBuffer = append(proxy, p.proxyBuffer...)
	p.complianceBuffer = append(compliance, p.complianceBuffer...)
	p.toolCallBuffer = append(toolCalls, p.toolCallBuffer...)

	// Trim to the cap, oldest first. Kinds are drained in order of how much
	// the hub loses without them: a proxy summary is a rollup of an analytics
	// event, a budget record is what spend enforcement runs on.
	dropped := 0
	if max := p.config.MaxBufferSize; max > 0 {
		trim := func(n int) int {
			over := p.totalBufferedLocked() - max
			if over <= 0 || n == 0 {
				return 0
			}
			if over > n {
				over = n
			}
			return over
		}
		if n := trim(len(p.proxyBuffer)); n > 0 {
			p.proxyBuffer = p.proxyBuffer[n:]
			dropped += n
		}
		if n := trim(len(p.analyticsBuffer)); n > 0 {
			p.analyticsBuffer = p.analyticsBuffer[n:]
			if n <= len(p.analyticsMetadata) {
				p.analyticsMetadata = p.analyticsMetadata[n:]
			}
			dropped += n
		}
		if n := trim(len(p.toolCallBuffer)); n > 0 {
			p.toolCallBuffer = p.toolCallBuffer[n:]
			dropped += n
		}
		if n := trim(len(p.complianceBuffer)); n > 0 {
			p.complianceBuffer = p.complianceBuffer[n:]
			dropped += n
		}
		if n := trim(len(p.budgetBuffer)); n > 0 {
			p.budgetBuffer = p.budgetBuffer[n:]
			dropped += n
		}
	}

	if expired > 0 || dropped > 0 {
		log.Warn().
			Int("expired", expired).
			Int("dropped_over_cap", dropped).
			Int("buffered", p.totalBufferedLocked()).
			Msg("Analytics records discarded while the control server is unreachable")
	}
}

// sendWithRetry sends one pulse, retrying transport failures up to MaxRetries
// times with retryWait between attempts. Once the plugin is stopping only the
// attempt in progress is made, so a shutdown does not sit through the ladder.
func (p *AnalyticsPulsePlugin) sendWithRetry(pulse *pb.AnalyticsPulse, sequenceNum uint64) (*pb.AnalyticsPulseResponse, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.config.TimeoutSeconds)*time.Second)
		resp, err := p.grpcClient.SendAnalyticsPulse(ctx, pulse)
		cancel()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Warn().
			Err(err).
			Uint64("sequence", sequenceNum).
			Int("attempt", attempt+1).
			Int("max_retries", p.config.MaxRetries).
			Msg("Failed to send analytics pulse")

		if attempt >= p.config.MaxRetries || p.stopping() {
			return nil, lastErr
		}

		var done <-chan struct{}
		if p.ctx != nil {
			done = p.ctx.Done()
		}
		select {
		case <-time.After(p.retryWait()):
		case <-done:
			return nil, lastErr
		}
	}
}

// sendPulse creates and sends an analytics pulse to control server. Data is
// only released once the hub has accepted it; a pulse the hub never received
// goes back into the buffers for the next one.
func (p *AnalyticsPulsePlugin) sendPulse() {
	if !p.sending.CompareAndSwap(false, true) {
		log.Debug().Msg("Analytics pulse already in flight - skipping")
		return
	}
	defer p.sending.Store(false)

	p.bufferMutex.Lock()
	snap := p.takeSnapshotLocked()
	if snap.empty() {
		p.bufferMutex.Unlock()
		log.Debug().Msg("No analytics data to pulse - skipping")
		return
	}
	sequenceNum := p.sequenceNumber
	p.sequenceNumber++
	p.bufferMutex.Unlock()

	// Build and send pulse
	pulse := p.buildPulseMessage(snap.analytics, snap.metadata, snap.budget, snap.proxy, snap.compliance, snap.toolCalls, sequenceNum)

	log.Debug().
		Uint64("sequence", sequenceNum).
		Int("analytics_events", len(snap.analytics)).
		Int("budget_events", len(snap.budget)).
		Int("proxy_summaries", len(snap.proxy)).
		Int("compliance_events", len(snap.compliance)).
		Int("tool_calls", len(snap.toolCalls)).
		Uint32("total_records", pulse.TotalRecords).
		Msg("Sending analytics pulse to control server")

	resp, err := p.sendWithRetry(pulse, sequenceNum)
	if err != nil {
		p.bufferMutex.Lock()
		p.lastError = err
		p.lastErrorTime = time.Now()
		p.restoreSnapshotLocked(snap, p.lastErrorTime)
		buffered := p.totalBufferedLocked()
		p.bufferMutex.Unlock()
		log.Error().
			Err(err).
			Uint64("sequence", sequenceNum).
			Int("buffered_for_retry", buffered).
			Msg("Analytics pulse not delivered - data kept for the next pulse")
		return
	}

	p.bufferMutex.Lock()
	p.lastError = nil
	p.bufferMutex.Unlock()

	p.totalPulsesSent++
	p.totalRecordsSent += uint64(pulse.TotalRecords)
	p.lastPulseTime = time.Now()

	if resp.Success {
		log.Debug().
			Uint64("sequence", sequenceNum).
			Uint64("processed_records", resp.ProcessedRecords).
			Msg("Analytics pulse sent successfully")
	} else {
		// The hub received the pulse and refused it. Retrying the same data
		// would be refused again, so it is not restored.
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
	complianceData []ComplianceEventBuffer,
	toolCallData []ToolCallBuffer,
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

		var failoverFrom uint32
		if event.FailoverFromLLMID != nil {
			failoverFrom = uint32(*event.FailoverFromLLMID)
		}

		analyticsEvents = append(analyticsEvents, &pb.AnalyticsEvent{
			RequestId:               event.RequestID,
			AppId:                   uint32(event.AppID),
			LlmId:                   llmID,
			FailoverFromLlmId: failoverFrom,
			FailoverAttempt:   uint32(event.FailoverAttempt),
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

	// Convert compliance events
	var complianceEvents []*pb.ComplianceEventProto
	for _, ce := range complianceData {
		complianceEvents = append(complianceEvents, &pb.ComplianceEventProto{
			AppId:       ce.AppID,
			UserId:      ce.UserID,
			LlmId:       ce.LLMID,
			FilterName:  ce.FilterName,
			FilterScope: ce.FilterScope,
			EventType:   ce.EventType,
			Severity:    ce.Severity,
			Description: ce.Description,
			Metadata:    ce.Metadata,
			Vendor:      ce.Vendor,
			ModelName:   ce.ModelName,
			Timestamp:   timestamppb.New(ce.Timestamp),
		})
	}

	// Convert tool calls
	var toolCalls []*pb.ToolCallProto
	for _, tc := range toolCallData {
		toolCalls = append(toolCalls, &pb.ToolCallProto{
			ToolId:      tc.ToolID,
			OperationId: tc.OperationID,
			ExecTimeMs:  tc.ExecTimeMs,
			Timestamp:   timestamppb.New(tc.Timestamp),
		})
	}

	totalRecords := uint32(len(analyticsEvents) + len(budgetEvents) + len(proxySummaries) + len(complianceEvents) + len(toolCalls))

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
		ComplianceEvents: complianceEvents,
		ToolCalls:        toolCalls,
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
	log.Debug().Str("plugin", "analytics_pulse").Msg("Stopping analytics pulse plugin")

	if p.cancel != nil {
		p.cancel()
	}

	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}

	// Send any remaining buffered data
	p.sendPulseNow()

	log.Debug().
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
		"current_buffer_size":   len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer) + len(p.complianceBuffer) + len(p.toolCallBuffer),
		"analytics_buffered":    len(p.analyticsBuffer),
		"budget_buffered":       len(p.budgetBuffer),
		"proxy_buffered":        len(p.proxyBuffer),
		"compliance_buffered":   len(p.complianceBuffer),
		"tool_calls_buffered":   len(p.toolCallBuffer),
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