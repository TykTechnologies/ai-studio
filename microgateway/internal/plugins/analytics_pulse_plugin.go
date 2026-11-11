// internal/plugins/analytics_pulse_plugin.go
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PulseClientSource hands out the client of the edge's current connection to
// the control server (the edge client's GetGRPCClient). The connection is
// replaced whenever the edge reconnects, so the pulse asks for it on every
// send instead of keeping one: a kept client belongs to a connection the
// reconnect has closed, and every pulse through it fails.
type PulseClientSource interface {
	GetGRPCClient() pb.ConfigurationSyncServiceClient
}

// AnalyticsPulsePlugin is a built-in data collection plugin that sends analytics data to control server
type AnalyticsPulsePlugin struct {
	config        *PulsePluginConfig
	edgeID        string
	edgeNamespace string
	clients       PulseClientSource

	// Buffered data
	analyticsBuffer    []database.AnalyticsEvent
	analyticsMetadata  []AnalyticsMetadata // Store additional data not in database.AnalyticsEvent
	budgetBuffer       []BudgetUsageBuffer
	proxyBuffer        []ProxyLogBuffer
	complianceBuffer   []ComplianceEventBuffer
	toolCallBuffer     []ToolCallBuffer
	bufferMutex        sync.RWMutex
	// held is the batch of a pulse that was not delivered, kept apart from
	// the buffers so it is sent again as the same batch: the same records
	// (less any that expired or were trimmed) under the same sequence
	// number. The hub stores a batch once by (edge ID, boot ID, sequence
	// number), so if it did store the first attempt and only its answer was
	// lost, the repeat is recognised. Merged with newer records under a new
	// number, as it used to be, those records were stored twice. Guarded by
	// bufferMutex; nil while the pulse that took it is in flight.
	held *heldPulse

	// bootID is drawn once per process and sent with every pulse: sequence
	// numbers start at 1 again after a restart, and the boot ID keeps the
	// batches of two runs apart.
	bootID string

	// Pulse management
	// timerMu guards pulseTimer: the timer's own callback, a buffer-full
	// flush and Stop all replace or stop it.
	timerMu         sync.Mutex
	pulseTimer      *time.Timer
	// flushQueued is set while a buffer-full flush is waiting to run, so a
	// full buffer starts one flush rather than one per record added.
	flushQueued atomic.Bool
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

	// capture is the agent tracing outbox (agent_trace_capture.go); nil
	// unless the Enterprise microgateway enabled capture.
	capture atomic.Pointer[captureOutbox]

	// oversizedDropped counts records dropped because one alone was larger
	// than a pulse may be.
	oversizedDropped atomic.Uint64

	// Statistics
	totalRecordsSent uint64
	totalPulsesSent  uint64
	lastError        error
	lastErrorTime    time.Time
}

// PulsePluginConfig holds configuration for the analytics pulse plugin
type PulsePluginConfig struct {
	IntervalSeconds             int      `json:"interval_seconds"`
	// MaxBatchSize is unused: the setting is accepted but nothing reads it
	// (a pulse is bounded by MaxBufferSize, see batchRecords).
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
	TraceSpanID  string // agent tracing gateway span id; empty while tracing is off
	ModelName    string
	Vendor       string
	RequestBody  string
	ResponseBody string
	// NoUsage marks a request that produced no usage (a block, an error).
	NoUsage bool
	// The requested model and the price multiplier, for the hub's chat
	// record (not stored on the edge's analytics row).
	RequestedModel  string
	PriceMultiplier *float64
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

// NewAnalyticsPulsePlugin creates a new built-in analytics pulse plugin.
// clients is asked for the control client on every send (see PulseClientSource).
func NewAnalyticsPulsePlugin(
	edgeID, edgeNamespace string,
	clients PulseClientSource,
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
		clients:        clients,
		ctx:            ctx,
		cancel:         cancel,
		sequenceNumber: 1,
		bootID:         uuid.NewString(),
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
		InteractionType:        pulseInteractionType(req.InteractionType),
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
		TotalTimeMS:            req.TotalTimeMS,

		ErrorMessage:           "",
		TimeStamp:              req.Timestamp,
		CreatedAt:              req.Timestamp,

		// Failover marker (nil / 0 for a primary attempt)
		FailoverFromLLMID:      req.FailoverFromLLMID,
		FailoverAttempt:        req.FailoverAttempt,

		// Routing decision (empty unless addressed to a router)
		RouterKind:             req.RouterKind,
		RouterSlug:             req.RouterSlug,
		RouterPoolName:         req.RouterPool,
		Route:                  req.Route,
		RouteReason:            req.RouteReason,
		RouterSourceModel:      req.RouterSourceModel,
		RouterTargetModel:      req.RouterTargetModel,
		RouterSelectionAlgo:    req.RouterSelectionAlgo,
		RouteScore:             req.RouteScore,
		ShadowRoute:            req.ShadowRoute,
		OnBehalfOf:             req.OnBehalfOf,
		ActingAgent:            req.ActingAgent,

		// Model class, and the decision counts of a decision call
		RequestKind:            req.RequestKind,
		DecisionQuestions:      req.DecisionQuestions,
		DecisionRefusals:       req.DecisionRefusals,

		// Vendor service tier the usage was billed at
		ServiceTier:            req.ServiceTier,

		// The gateway's request id and the caller's
		GatewayRequestID:       req.GatewayRequestID,
		ClientRequestID:        req.ClientRequestID,
	}

	// Store metadata for pulse transmission
	metadata := AnalyticsMetadata{
		RequestID:    req.RequestID,
		TraceSpanID:  req.TraceSpanID,
		ModelName:    req.ModelName,
		Vendor:       req.Vendor,
		RequestBody:  req.RequestBody,  // From analytics interface
		ResponseBody: req.ResponseBody, // From analytics interface
		NoUsage:      req.NoUsage,

		RequestedModel:  req.RequestedModel,
		PriceMultiplier: req.PriceMultiplier,
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
		} else if p.flushQueued.CompareAndSwap(false, true) {
			log.Warn().
				Int("total_buffered", totalBuffered).
				Int("max_buffer_size", p.config.MaxBufferSize).
				Msg("Analytics buffer is full, triggering immediate pulse")
			go p.flushFullBuffer()
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
	p.timerMu.Lock()
	defer p.timerMu.Unlock()
	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}
	if p.stopping() {
		p.pulseTimer = nil
		return
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
	p.stopTimer()
	p.sendPulse()
	p.schedulePulse()
}

func (p *AnalyticsPulsePlugin) stopTimer() {
	p.timerMu.Lock()
	defer p.timerMu.Unlock()
	if p.pulseTimer != nil {
		p.pulseTimer.Stop()
	}
}

// flushFullBuffer sends a pulse for a full buffer. A pulse already in flight
// is waited for, not skipped: the records that filled the buffer arrived after
// it took its snapshot.
func (p *AnalyticsPulsePlugin) flushFullBuffer() {
	defer p.flushQueued.Store(false)
	for p.sending.Load() && !p.stopping() {
		time.Sleep(10 * time.Millisecond)
	}
	p.sendPulseNow()
}

// pulseSnapshot is one pulse's worth of buffered data, taken out of the
// buffers under the lock so it can be sent without holding them. If the send
// fails it is held (heldPulse) and sent again before whatever arrived in the
// meantime.
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

// heldPulse is an undelivered batch and the sequence number it was sent under.
type heldPulse struct {
	snap pulseSnapshot
	seq  uint64
}

func (s *pulseSnapshot) size() int {
	return len(s.analytics) + len(s.budget) + len(s.proxy) + len(s.compliance) + len(s.toolCalls)
}

// totalBufferedLocked counts every record waiting for a pulse: the buffers
// and the batch held for a retry. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) totalBufferedLocked() int {
	n := len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer) +
		len(p.complianceBuffer) + len(p.toolCallBuffer)
	if p.held != nil {
		n += p.held.snap.size()
	}
	return n
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

// heldSizeLocked is the number of records in the batch held for a retry.
// Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) heldSizeLocked() int {
	if p.held == nil {
		return 0
	}
	return p.held.snap.size()
}

// stopping reports whether the plugin has been told to shut down.
func (p *AnalyticsPulsePlugin) stopping() bool {
	return p.ctx != nil && p.ctx.Err() != nil
}

// maxPulseBytes is the largest pulse message the edge sends. The control
// plane refuses a message above its gRPC receive limit (16 MiB by default)
// before any handler sees it, so a pulse above that can never be delivered;
// half of it leaves room for a control plane set lower and for the
// difference between this estimate and the bytes on the wire. A variable
// so that tests can make it small.
var maxPulseBytes = 8 << 20

// batchRecords is the most records one pulse carries: MaxBufferSize, the
// size at which a filling buffer is flushed, so the largest pulse normal
// operation sends. A backlog (what gathered while pulses failed, or while
// one slow pulse was in flight) is sent as several pulses of that size, one
// after the other, not as one message of whatever size it has reached: the
// control plane stores a pulse in one transaction, and one of a quarter of
// a million records neither fits its message limit nor finishes within the
// pulse timeout. 0 means no bound.
func (p *AnalyticsPulsePlugin) batchRecords() int {
	return p.config.MaxBufferSize
}

// takeBatchLocked moves up to max buffered records into a snapshot (all of
// them when max is 0 or they are fewer), the oldest of each kind first and
// each kind in proportion to what is buffered of it, so that no kind waits
// behind another. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) takeBatchLocked(max int) pulseSnapshot {
	total := len(p.analyticsBuffer) + len(p.budgetBuffer) + len(p.proxyBuffer) + len(p.complianceBuffer) + len(p.toolCallBuffer)
	if max <= 0 || total <= max {
		return p.takeSnapshotLocked()
	}
	// share is how many of n records of one kind go into this batch.
	share := func(n int) int {
		k := (n*max + total - 1) / total // rounded up, so a kind with records gets at least one
		if k > n {
			k = n
		}
		return k
	}
	var snap pulseSnapshot
	n := share(len(p.analyticsBuffer))
	snap.analytics, p.analyticsBuffer = p.analyticsBuffer[:n:n], p.analyticsBuffer[n:]
	if n > len(p.analyticsMetadata) {
		n = len(p.analyticsMetadata)
	}
	snap.metadata, p.analyticsMetadata = p.analyticsMetadata[:n:n], p.analyticsMetadata[n:]
	n = share(len(p.budgetBuffer))
	snap.budget, p.budgetBuffer = p.budgetBuffer[:n:n], p.budgetBuffer[n:]
	n = share(len(p.proxyBuffer))
	snap.proxy, p.proxyBuffer = p.proxyBuffer[:n:n], p.proxyBuffer[n:]
	n = share(len(p.complianceBuffer))
	snap.compliance, p.complianceBuffer = p.complianceBuffer[:n:n], p.complianceBuffer[n:]
	n = share(len(p.toolCallBuffer))
	snap.toolCalls, p.toolCallBuffer = p.toolCallBuffer[:n:n], p.toolCallBuffer[n:]
	return snap
}

// halve splits a snapshot in two: the first gets half of the records
// (rounded down, at least one), the oldest of each kind, kinds in a fixed
// order; the second gets the rest. A snapshot of one record cannot be
// split: second is then empty.
func (s pulseSnapshot) halve() (first, second pulseSnapshot) {
	if s.size() <= 1 {
		return s, pulseSnapshot{}
	}
	left := s.size() / 2
	// take says how many of n records of the next kind the first half gets.
	take := func(n int) int {
		k := n
		if k > left {
			k = left
		}
		left -= k
		return k
	}
	a := take(len(s.analytics))
	first.analytics, second.analytics = s.analytics[:a:a], s.analytics[a:]
	// Analytics events and their metadata are paired by index.
	m := a
	if m > len(s.metadata) {
		m = len(s.metadata)
	}
	first.metadata, second.metadata = s.metadata[:m:m], s.metadata[m:]
	b := take(len(s.budget))
	first.budget, second.budget = s.budget[:b:b], s.budget[b:]
	c := take(len(s.proxy))
	first.proxy, second.proxy = s.proxy[:c:c], s.proxy[c:]
	d := take(len(s.compliance))
	first.compliance, second.compliance = s.compliance[:d:d], s.compliance[d:]
	e := take(len(s.toolCalls))
	first.toolCalls, second.toolCalls = s.toolCalls[:e:e], s.toolCalls[e:]
	return first, second
}

// putBackLocked returns records taken for a pulse to the front of the
// buffers: they are older than anything buffered since. Caller holds
// bufferMutex.
func (p *AnalyticsPulsePlugin) putBackLocked(s pulseSnapshot) {
	if s.empty() {
		return
	}
	// Analytics events and their metadata are paired by index.
	meta := s.metadata
	for len(meta) < len(s.analytics) {
		meta = append(meta, AnalyticsMetadata{})
	}
	p.analyticsBuffer = append(append(make([]database.AnalyticsEvent, 0, len(s.analytics)+len(p.analyticsBuffer)), s.analytics...), p.analyticsBuffer...)
	p.analyticsMetadata = append(append(make([]AnalyticsMetadata, 0, len(meta)+len(p.analyticsMetadata)), meta...), p.analyticsMetadata...)
	p.budgetBuffer = append(append(make([]BudgetUsageBuffer, 0, len(s.budget)+len(p.budgetBuffer)), s.budget...), p.budgetBuffer...)
	p.proxyBuffer = append(append(make([]ProxyLogBuffer, 0, len(s.proxy)+len(p.proxyBuffer)), s.proxy...), p.proxyBuffer...)
	p.complianceBuffer = append(append(make([]ComplianceEventBuffer, 0, len(s.compliance)+len(p.complianceBuffer)), s.compliance...), p.complianceBuffer...)
	p.toolCallBuffer = append(append(make([]ToolCallBuffer, 0, len(s.toolCalls)+len(p.toolCallBuffer)), s.toolCalls...), p.toolCallBuffer...)
}

// takeSnapshotLocked moves everything buffered into a snapshot. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) takeSnapshotLocked() pulseSnapshot {
	// The buffers themselves become the snapshot and fresh ones start empty:
	// copying up to MaxBufferSize large records here held the lock every
	// recording request waits on.
	snap := pulseSnapshot{
		analytics:  p.analyticsBuffer,
		metadata:   p.analyticsMetadata,
		budget:     p.budgetBuffer,
		proxy:      p.proxyBuffer,
		compliance: p.complianceBuffer,
		toolCalls:  p.toolCallBuffer,
	}
	p.analyticsBuffer = nil
	p.analyticsMetadata = nil
	p.budgetBuffer = nil
	p.proxyBuffer = nil
	p.complianceBuffer = nil
	p.toolCallBuffer = nil
	return snap
}

// nextBatchLocked returns the batch to send next: the held one, exactly as
// it was left, under the sequence number it was first sent with; otherwise
// the oldest buffered records, at most batchRecords of them, under a new
// number. retried says which. An empty
// snapshot means there is nothing to send. Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) nextBatchLocked() (snap pulseSnapshot, seq uint64, retried bool) {
	if h := p.held; h != nil {
		p.held = nil
		return h.snap, h.seq, true
	}
	snap = p.takeBatchLocked(p.batchRecords())
	if snap.empty() {
		return snap, 0, false
	}
	seq = p.sequenceNumber
	p.sequenceNumber++
	return snap, seq, false
}

// holdSnapshotLocked keeps an undelivered batch for the next pulse, which
// sends it again under seq before anything newer. Records that have aged
// past EdgeRetentionHours are dropped from it, and the held batch and the
// buffers together are trimmed to MaxBufferSize by discarding the oldest
// records (the held batch's first). A batch only ever loses records while
// it is held: nothing newer is added to it, so the hub, which stores a
// batch once by its sequence number, can tell a repeat from new data.
// Caller holds bufferMutex.
func (p *AnalyticsPulsePlugin) holdSnapshotLocked(snap pulseSnapshot, seq uint64, now time.Time) {
	cutoff := now.Add(-time.Duration(p.config.EdgeRetentionHours) * time.Hour)
	fresh := func(ts time.Time) bool { return p.config.EdgeRetentionHours <= 0 || !ts.Before(cutoff) }

	// Analytics events and their metadata are paired by index; keep them in step.
	kept := pulseSnapshot{
		analytics: make([]database.AnalyticsEvent, 0, len(snap.analytics)),
		metadata:  make([]AnalyticsMetadata, 0, len(snap.analytics)),
	}
	for i, ev := range snap.analytics {
		if !fresh(ev.TimeStamp) {
			continue
		}
		kept.analytics = append(kept.analytics, ev)
		if i < len(snap.metadata) {
			kept.metadata = append(kept.metadata, snap.metadata[i])
		} else {
			kept.metadata = append(kept.metadata, AnalyticsMetadata{})
		}
	}
	for _, b := range snap.budget {
		if fresh(b.Timestamp) {
			kept.budget = append(kept.budget, b)
		}
	}
	for _, pr := range snap.proxy {
		if fresh(pr.LastRequest) {
			kept.proxy = append(kept.proxy, pr)
		}
	}
	for _, ce := range snap.compliance {
		if fresh(ce.Timestamp) {
			kept.compliance = append(kept.compliance, ce)
		}
	}
	for _, tc := range snap.toolCalls {
		if fresh(tc.Timestamp) {
			kept.toolCalls = append(kept.toolCalls, tc)
		}
	}
	expired := snap.size() - kept.size()
	p.held = &heldPulse{snap: kept, seq: seq}

	// Trim to the cap, oldest first: the held batch is older than anything
	// buffered since. Kinds are drained in order of how much the hub loses
	// without them: a proxy summary is a rollup of an analytics event, a
	// budget record is what spend enforcement runs on.
	dropped := 0
	if max := p.config.MaxBufferSize; max > 0 {
		// trim says how many of n records to drop to get back under the cap.
		trim := func(n int) int {
			over := p.totalBufferedLocked() - max
			if over <= 0 || n == 0 {
				return 0
			}
			if over > n {
				over = n
			}
			dropped += over
			return over
		}
		h := &p.held.snap
		h.proxy = h.proxy[trim(len(h.proxy)):]
		p.proxyBuffer = p.proxyBuffer[trim(len(p.proxyBuffer)):]
		if n := trim(len(h.analytics)); n > 0 {
			h.analytics, h.metadata = h.analytics[n:], h.metadata[n:]
		}
		if n := trim(len(p.analyticsBuffer)); n > 0 {
			p.analyticsBuffer = p.analyticsBuffer[n:]
			if n <= len(p.analyticsMetadata) {
				p.analyticsMetadata = p.analyticsMetadata[n:]
			}
		}
		h.toolCalls = h.toolCalls[trim(len(h.toolCalls)):]
		p.toolCallBuffer = p.toolCallBuffer[trim(len(p.toolCallBuffer)):]
		h.compliance = h.compliance[trim(len(h.compliance)):]
		p.complianceBuffer = p.complianceBuffer[trim(len(p.complianceBuffer)):]
		h.budget = h.budget[trim(len(h.budget)):]
		p.budgetBuffer = p.budgetBuffer[trim(len(p.budgetBuffer)):]
	}
	// Nothing left of it: there is no batch to send again.
	if p.held.snap.empty() {
		p.held = nil
	}

	pulseDroppedOverCap.Add(uint64(dropped))
	pulseDroppedExpired.Add(uint64(expired))
	if expired > 0 || dropped > 0 {
		log.Warn().
			Int("expired", expired).
			Int("dropped_over_cap", dropped).
			Int("buffered", p.totalBufferedLocked()).
			Int("max_buffer_size", p.config.MaxBufferSize).
			Str("pulses_fail_because", pulseFailureReason(p.lastError)).
			AnErr("last_error", p.lastError).
			Msg("Analytics records discarded: pulses are failing and the edge keeps at most max_buffer_size records for the next one")
	}
}

// Records the pulse gave up, by reason, since the process started
// (microgateway_analytics_pulse_dropped_records_total). They are lost to the
// control plane: its analytics, and the spend it computes from them, are
// that much short.
var (
	// pulseDroppedOverCap: discarded, oldest first, to keep the held batch
	// and the buffers within max_buffer_size while pulses fail.
	pulseDroppedOverCap atomic.Uint64
	// pulseDroppedExpired: held for longer than edge_retention_hours.
	pulseDroppedExpired atomic.Uint64
	// pulseDroppedOversized: one record larger than a pulse may be.
	pulseDroppedOversized atomic.Uint64
)

// PulseDroppedRecords returns how many records the analytics pulse has
// given up since the process started, by reason: "over_cap", "expired",
// "oversized".
func PulseDroppedRecords() map[string]uint64 {
	return map[string]uint64{
		"over_cap":  pulseDroppedOverCap.Load(),
		"expired":   pulseDroppedExpired.Load(),
		"oversized": pulseDroppedOversized.Load(),
	}
}

// pulseFailureReason says, for the log, why a pulse failed: the edge keeps
// records the same way in each case, but an operator reading "discarded"
// needs to know whether the control plane was away or answered and could
// not store.
func pulseFailureReason(err error) string {
	switch {
	case err == nil:
		return "unknown"
	case errors.Is(err, errNotConnected):
		return "the edge has no connection to a control plane"
	case status.Code(err) == codes.Unavailable && strings.Contains(err.Error(), "not stored"):
		return "the control plane answered but could not store the pulse (its database is unavailable)"
	case status.Code(err) == codes.Unavailable:
		return "the control plane could not be reached"
	case status.Code(err) == codes.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded):
		return "the control plane did not answer within timeout_seconds"
	default:
		return "the control plane refused or failed the call"
	}
}

// errNotConnected fails a pulse attempt while the edge has no connection to
// the control server (between a dropped connection and the reconnect).
var errNotConnected = errors.New("not connected to the control server")

// isMessageTooLarge reports whether err is the control plane (or this
// edge's own client) refusing a message for its size: gRPC's
// ResourceExhausted, "received message larger than max" or "trying to send
// message larger than max". The control plane is reachable; it is the
// message that cannot be delivered.
func isMessageTooLarge(err error) bool {
	return status.Code(err) == codes.ResourceExhausted && strings.Contains(err.Error(), "larger than max")
}

// sendWithRetry sends one pulse, retrying transport failures up to MaxRetries
// times with retryWait between attempts. Once the plugin is stopping only the
// attempt in progress is made, so a shutdown does not sit through the ladder.
func (p *AnalyticsPulsePlugin) sendWithRetry(pulse *pb.AnalyticsPulse, sequenceNum uint64) (*pb.AnalyticsPulseResponse, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		var resp *pb.AnalyticsPulseResponse
		var err error
		if client := p.clients.GetGRPCClient(); client == nil {
			err = errNotConnected
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.config.TimeoutSeconds)*time.Second)
			resp, err = client.SendAnalyticsPulse(ctx, pulse)
			cancel()
		}
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

		// The same message would be refused for its size again.
		if attempt >= p.config.MaxRetries || p.stopping() || isMessageTooLarge(err) {
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
// only released once the hub has accepted it; a pulse the hub never
// acknowledged is held and sent again, as the same batch, by the next one.
// When that held batch is delivered, what was buffered meanwhile follows at
// once as a batch of its own.
func (p *AnalyticsPulsePlugin) sendPulse() {
	if !p.sending.CompareAndSwap(false, true) {
		log.Debug().Msg("Analytics pulse already in flight - skipping")
		return
	}
	defer p.sending.Store(false)

	for {
		p.bufferMutex.Lock()
		snap, sequenceNum, retried := p.nextBatchLocked()
		p.bufferMutex.Unlock()
		if snap.empty() {
			log.Debug().Msg("No analytics data to pulse - skipping")
			return
		}
		switch p.deliver(snap, sequenceNum, retried) {
		case pulseFailed:
			return
		case pulseSplit:
			// Part of the batch is held under its number and the rest is
			// back in the buffers: send on at once.
			continue
		}
		if retried {
			// The held batch is through: what was buffered meanwhile
			// follows at once as batches of its own.
			continue
		}
		// A backlog larger than one pulse is sent pulse after pulse; what
		// is left below a full one waits for the timer, as always.
		p.bufferMutex.Lock()
		more := p.batchRecords() > 0 && p.totalBufferedLocked() >= p.batchRecords()
		p.bufferMutex.Unlock()
		if !more || p.stopping() {
			return
		}
	}
}

// pulseOutcome is what became of one batch.
type pulseOutcome int

const (
	// pulseDelivered: the control plane answered (stored it, or refused the
	// data itself, which a repeat would not change).
	pulseDelivered pulseOutcome = iota
	// pulseFailed: not delivered; the batch is held for the next pulse.
	pulseFailed
	// pulseSplit: the message was too large to deliver; the older half of
	// the batch is held under its sequence number and the newer half is
	// back in the buffers.
	pulseSplit
)

// splitTooLarge deals with a batch whose message is too large to deliver:
// its older half is held under the batch's sequence number, to be sent at
// once, and its newer half goes back to the front of the buffers, to be
// sent under a number of its own. Nothing is discarded for it, and the
// buffer cap is not applied: the control plane is reachable. A single
// record that is too large by itself can never be delivered and is dropped,
// with an error in the log.
//
// The sequence number stays with the older half. The control plane stores
// a batch once by that number, and a batch refused for its size was never
// stored under it: a message is refused before anything is read from it.
func (p *AnalyticsPulsePlugin) splitTooLarge(snap pulseSnapshot, sequenceNum uint64, size int, cause string) pulseOutcome {
	first, second := snap.halve()
	if second.empty() {
		p.oversizedDropped.Add(uint64(first.size()))
		pulseDroppedOversized.Add(uint64(first.size()))
		log.Error().
			Uint64("sequence", sequenceNum).
			Int("message_bytes", size).
			Int("limit_bytes", maxPulseBytes).
			Str("cause", cause).
			Msg("Analytics pulse: one record is larger than a pulse may be and cannot be delivered; it is dropped (stored request and response bodies are the usual reason: see ANALYTICS_STORE_REQUESTS / ANALYTICS_STORE_RESPONSES)")
		return pulseDelivered
	}
	p.bufferMutex.Lock()
	p.held = &heldPulse{snap: first, seq: sequenceNum}
	p.putBackLocked(second)
	p.bufferMutex.Unlock()
	log.Warn().
		Uint64("sequence", sequenceNum).
		Int("message_bytes", size).
		Int("limit_bytes", maxPulseBytes).
		Int("records", snap.size()).
		Int("sending_first", first.size()).
		Str("cause", cause).
		Msg("Analytics pulse too large for one message - sending it in parts")
	return pulseSplit
}

// deliver sends one batch under sequenceNum and reports what became of it.
// On failure the batch is held for the next pulse.
func (p *AnalyticsPulsePlugin) deliver(snap pulseSnapshot, sequenceNum uint64, retried bool) pulseOutcome {
	// Build and send pulse
	pulse := p.buildPulseMessage(snap.analytics, snap.metadata, snap.budget, snap.proxy, snap.compliance, snap.toolCalls, sequenceNum)
	if n := makeValidUTF8(pulse.ProtoReflect()); n > 0 {
		log.Warn().
			Uint64("sequence", sequenceNum).
			Int("strings", n).
			Msg("Analytics pulse: replaced invalid UTF-8 so the pulse can be sent")
	}
	// Records are counted, not weighed, when a batch is taken: a batch of
	// records with large bodies can still be too large for one message.
	if size := proto.Size(pulse); size > maxPulseBytes {
		return p.splitTooLarge(snap, sequenceNum, size, "over the edge's own limit")
	}

	log.Debug().
		Uint64("sequence", sequenceNum).
		Bool("retried_batch", retried).
		Int("analytics_events", len(snap.analytics)).
		Int("budget_events", len(snap.budget)).
		Int("proxy_summaries", len(snap.proxy)).
		Int("compliance_events", len(snap.compliance)).
		Int("tool_calls", len(snap.toolCalls)).
		Uint32("total_records", pulse.TotalRecords).
		Msg("Sending analytics pulse to control server")

	resp, err := p.sendWithRetry(pulse, sequenceNum)
	if err != nil && isMessageTooLarge(err) {
		// The control plane's limit is below this edge's: it answered, so
		// this is not an outage, and nothing is discarded for it.
		return p.splitTooLarge(snap, sequenceNum, proto.Size(pulse), err.Error())
	}
	if err != nil {
		p.bufferMutex.Lock()
		p.lastError = err
		p.lastErrorTime = time.Now()
		p.holdSnapshotLocked(snap, sequenceNum, p.lastErrorTime)
		buffered := p.totalBufferedLocked()
		p.bufferMutex.Unlock()
		log.Error().
			Err(err).
			Uint64("sequence", sequenceNum).
			Int("buffered_for_retry", buffered).
			Msg("Analytics pulse not delivered - data kept for the next pulse")
		return pulseFailed
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
		// would be refused again, so it is not held.
		log.Error().
			Str("message", resp.Message).
			Uint64("sequence", sequenceNum).
			Msg("Analytics pulse rejected by control server")
	}
	return pulseDelivered
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
			TraceSpanId:             eventMetadata.TraceSpanID,
			AppId:                   uint32(event.AppID),
			LlmId:                   llmID,
			FailoverFromLlmId: failoverFrom,
			FailoverAttempt:   uint32(event.FailoverAttempt),
			RouterKind:        event.RouterKind,
			RouterSlug:        event.RouterSlug,
			RouterPool:        event.RouterPoolName,
			Route:             event.Route,
			RouteReason:       event.RouteReason,
			RouteSourceModel:  event.RouterSourceModel,
			RouteTargetModel:  event.RouterTargetModel,
			RouteSelection:    event.RouterSelectionAlgo,
			RouteScore:        event.RouteScore,
			ShadowRoute:       event.ShadowRoute,
			OnBehalfOf:        event.OnBehalfOf,
			ActingAgent:       event.ActingAgent,
			RequestKind:       event.RequestKind,
			DecisionQuestions: uint32(event.DecisionQuestions),
			DecisionRefusals:  uint32(event.DecisionRefusals),
			ServiceTier:       event.ServiceTier,
			GatewayRequestId:  event.GatewayRequestID,
			ClientRequestId:   event.ClientRequestID,
			NoUsage:           eventMetadata.NoUsage,
			RequestedModel:    eventMetadata.RequestedModel,
			PriceMultiplier:   eventMetadata.PriceMultiplier,
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
		BootId:           p.bootID,
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

	p.stopTimer()

	// Send any remaining buffered data. A pulse still in flight only carries
	// its own snapshot, and sendPulse skips while one is in flight, so wait
	// for it (the cancel above cuts its retry ladder short) before sending
	// what arrived since.
	deadline := time.Now().Add(time.Duration(p.config.TimeoutSeconds+1) * time.Second)
	for p.sending.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	p.sendPulse()
	p.stopCapture()

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
		"current_buffer_size":   p.totalBufferedLocked(),
		"analytics_buffered":    len(p.analyticsBuffer),
		"budget_buffered":       len(p.budgetBuffer),
		"proxy_buffered":        len(p.proxyBuffer),
		"compliance_buffered":   len(p.complianceBuffer),
		"tool_calls_buffered":   len(p.toolCallBuffer),
		"held_for_retry":        p.heldSizeLocked(),
		"oversized_dropped":     p.oversizedDropped.Load(),
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

// pulseInteractionType is the interaction type an analytics event carries to
// the hub: a guardrail's judge call keeps its own type (charged to the judge
// LLM, not an App); everything else on an edge is proxy traffic.
func pulseInteractionType(t string) string {
	if t == string(models.GuardrailJudgeInteraction) {
		return t
	}
	return "proxy"
}
