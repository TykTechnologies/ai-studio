package metrics

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Agent Call Tracing agent and tracing-health metrics (spec §5.7; recorded
// by the Enterprise assembler and exporter). The GenAI token, duration and
// TTFT metrics stay in genai.go and are not re-emitted.
//
// No session, trace, user or tool-call id is ever an attribute: the only
// attributes are tyk.app.id (switchable), currency, confidence, rule and
// exporter.
//
// Every instrument is registered once per meter: on the Prometheus meter by
// Init, and on any meter the Enterprise exporter adds for OTLP metrics
// (AddAgentMetricsMeter).
const (
	attrAgentApp      = "tyk.app.id"
	attrAgentCurrency = "currency"
	attrConfidence    = "confidence"
	attrRule          = "rule"
	attrExporter      = "exporter"
)

type agentInstruments struct {
	sessionDuration otelmetric.Float64Histogram
	turnLLMCalls    otelmetric.Int64Histogram
	turnToolCalls   otelmetric.Int64Histogram
	cost            otelmetric.Int64Counter
	links           otelmetric.Int64Counter
	exportFailures  otelmetric.Int64Counter
}

var (
	agentSetsMu sync.RWMutex
	agentSets   []*agentInstruments

	agentIncludeApp atomic.Bool
	// agentMetricsOff switches off the tyk_agent_* metrics
	// (AGENT_TRACING_METRICS_ENABLED=false); the tyk_tracing_* health
	// metrics stay.
	agentMetricsOff atomic.Bool

	// activeSessionsFn reports the active session count, and whether this
	// node should report it (only the assembler's leader does).
	activeSessionsFn atomic.Pointer[func() (int64, bool)]

	coverageMu sync.Mutex
	coverage   = map[uint]*[2]int64{} // app -> {LLM calls, LLM calls with attribution >= exact}
)

func init() { agentIncludeApp.Store(true) }

// SetAgentMetricsIncludeAppID switches the tyk.app.id attribute
// (AGENT_TRACING_METRICS_INCLUDE_APP_ID).
func SetAgentMetricsIncludeAppID(on bool) { agentIncludeApp.Store(on) }

// SetAgentMetricsEnabled switches the tyk_agent_* metrics
// (AGENT_TRACING_METRICS_ENABLED, default on).
func SetAgentMetricsEnabled(on bool) { agentMetricsOff.Store(!on) }

// SetActiveSessionsFn sets the function the active sessions gauge reads.
func SetActiveSessionsFn(fn func() (int64, bool)) {
	if fn == nil {
		activeSessionsFn.Store(nil)
		return
	}
	activeSessionsFn.Store(&fn)
}

// AddAgentMetricsMeter registers the agent instruments on another meter (an
// OTLP metric exporter's), so every recording reaches it too.
func AddAgentMetricsMeter(meter otelmetric.Meter) error {
	set, err := newAgentInstruments(meter)
	if err != nil {
		return err
	}
	agentSetsMu.Lock()
	agentSets = append(agentSets, set)
	agentSetsMu.Unlock()
	return nil
}

func initAgentMetrics(meter otelmetric.Meter) {
	set, err := newAgentInstruments(meter)
	if err != nil {
		panic("failed to create agent tracing metrics: " + err.Error())
	}
	agentSetsMu.Lock()
	// Init may run again (tests); the Prometheus set is always the first.
	agentSets = []*agentInstruments{set}
	agentSetsMu.Unlock()
}

func newAgentInstruments(meter otelmetric.Meter) (*agentInstruments, error) {
	var s agentInstruments
	var err error
	if s.sessionDuration, err = meter.Float64Histogram("tyk_agent_session_duration",
		otelmetric.WithUnit("s"), otelmetric.WithDescription("Duration of closed agent sessions")); err != nil {
		return nil, err
	}
	if s.turnLLMCalls, err = meter.Int64Histogram("tyk_agent_turn_llm_calls",
		otelmetric.WithDescription("LLM calls per closed agent turn")); err != nil {
		return nil, err
	}
	if s.turnToolCalls, err = meter.Int64Histogram("tyk_agent_turn_tool_calls",
		otelmetric.WithDescription("Tool calls per closed agent turn")); err != nil {
		return nil, err
	}
	if s.cost, err = meter.Int64Counter("tyk_agent_cost",
		otelmetric.WithDescription("Cost of agent LLM calls in micro-units of currency")); err != nil {
		return nil, err
	}
	if s.links, err = meter.Int64Counter("tyk_tracing_correlation_links",
		otelmetric.WithDescription("Correlation edges written, by confidence and rule")); err != nil {
		return nil, err
	}
	if s.exportFailures, err = meter.Int64Counter("tyk_tracing_export_failures",
		otelmetric.WithDescription("Failed OTel span exports, by exporter")); err != nil {
		return nil, err
	}
	if _, err = meter.Int64ObservableGauge("tyk_agent_sessions_active",
		otelmetric.WithDescription("Agent sessions currently active (reported by the assembler's leader)"),
		otelmetric.WithInt64Callback(func(_ context.Context, o otelmetric.Int64Observer) error {
			if fn := activeSessionsFn.Load(); fn != nil && !agentMetricsOff.Load() {
				if n, ok := (*fn)(); ok {
					o.Observe(n)
				}
			}
			return nil
		})); err != nil {
		return nil, err
	}
	if _, err = meter.Float64ObservableGauge("tyk_tracing_coverage_ratio",
		otelmetric.WithDescription("Share of LLM calls with session attribution of at least exact confidence, per App, since start"),
		otelmetric.WithFloat64Callback(func(_ context.Context, o otelmetric.Float64Observer) error {
			coverageMu.Lock()
			defer coverageMu.Unlock()
			for app, c := range coverage {
				if c[0] > 0 {
					o.Observe(float64(c[1])/float64(c[0]), appOpt(app)...)
				}
			}
			return nil
		})); err != nil {
		return nil, err
	}
	return &s, nil
}

func agentEach(fn func(*agentInstruments)) {
	if !initialized.Load() {
		return
	}
	agentSetsMu.RLock()
	defer agentSetsMu.RUnlock()
	for _, s := range agentSets {
		fn(s)
	}
}

func appOpt(appID uint) []otelmetric.ObserveOption {
	if !agentIncludeApp.Load() {
		return nil
	}
	return []otelmetric.ObserveOption{otelmetric.WithAttributes(attribute.String(attrAgentApp, strconv.FormatUint(uint64(appID), 10)))}
}

func appAttrs(appID uint, extra ...attribute.KeyValue) otelmetric.MeasurementOption {
	if agentIncludeApp.Load() {
		extra = append(extra, attribute.String(attrAgentApp, strconv.FormatUint(uint64(appID), 10)))
	}
	return otelmetric.WithAttributes(extra...)
}

// RecordAgentSessionClosed records a closed session's duration.
func RecordAgentSessionClosed(ctx context.Context, appID uint, seconds float64) {
	if agentMetricsOff.Load() {
		return
	}
	agentEach(func(s *agentInstruments) { s.sessionDuration.Record(ctx, seconds, appAttrs(appID)) })
}

// RecordAgentTurnClosed records a closed turn's LLM and tool call counts.
func RecordAgentTurnClosed(ctx context.Context, appID uint, llmCalls, toolCalls int) {
	if agentMetricsOff.Load() {
		return
	}
	agentEach(func(s *agentInstruments) {
		s.turnLLMCalls.Record(ctx, int64(llmCalls), appAttrs(appID))
		s.turnToolCalls.Record(ctx, int64(toolCalls), appAttrs(appID))
	})
}

// RecordAgentCost adds micros of cost in currency.
func RecordAgentCost(ctx context.Context, appID uint, currency string, micros int64) {
	if micros <= 0 || agentMetricsOff.Load() {
		return
	}
	if currency == "" {
		currency = "USD"
	}
	agentEach(func(s *agentInstruments) {
		s.cost.Add(ctx, micros, appAttrs(appID, attribute.String(attrAgentCurrency, currency)))
	})
}

// RecordCorrelationLink counts one correlation edge.
func RecordCorrelationLink(ctx context.Context, confidence, rule string) {
	agentEach(func(s *agentInstruments) {
		s.links.Add(ctx, 1, otelmetric.WithAttributes(attribute.String(attrConfidence, confidence), attribute.String(attrRule, rule)))
	})
}

// RecordExportFailure counts a failed OTel span export.
func RecordExportFailure(ctx context.Context, exporter string) {
	agentEach(func(s *agentInstruments) {
		s.exportFailures.Add(ctx, 1, otelmetric.WithAttributes(attribute.String(attrExporter, exporter)))
	})
}

// RecordCoverage counts one LLM call of an App and whether its session
// attribution was at least exact (declared, or an exact or declared edge).
func RecordCoverage(appID uint, attributed bool) {
	coverageMu.Lock()
	defer coverageMu.Unlock()
	c := coverage[appID]
	if c == nil {
		c = &[2]int64{}
		coverage[appID] = c
	}
	c[0]++
	if attributed {
		c[1]++
	}
}
