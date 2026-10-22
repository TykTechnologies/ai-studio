package tracing

import (
	"net/http"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/baggage"
)

// W3C trace context header names.
const (
	headerTraceparent = "Traceparent"
	headerTracestate  = "Tracestate"
	headerBaggage     = "Baggage"
)

// baggageAllowlist holds the W3C baggage keys that may leave the gateway
// (AGENT_TRACING_BAGGAGE_ALLOWLIST, default session.id). Client baggage used
// to reach LLM vendors and tool upstreams verbatim; anything not listed is
// now removed in every edition (decision Q11).
var baggageAllowlist atomic.Pointer[map[string]struct{}]

func init() { SetBaggageAllowlist([]string{"session.id"}) }

// SetBaggageAllowlist sets the baggage keys forwarded upstream. An empty
// list forwards no baggage at all.
func SetBaggageAllowlist(keys []string) {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			m[k] = struct{}{}
		}
	}
	baggageAllowlist.Store(&m)
}

// FilterBaggage reduces the baggage header in h to the allowlisted members.
// A header that does not parse is removed whole (fail closed). Keys set
// without canonical case (a raw map write) are folded in. It allocates only
// when a baggage header is present.
func FilterBaggage(h http.Header) {
	values := h[headerBaggage]
	for k, v := range h {
		if k != headerBaggage && strings.EqualFold(k, headerBaggage) {
			values = append(values, v...)
			delete(h, k)
		}
	}
	if len(values) == 0 {
		return
	}
	allowed := *baggageAllowlist.Load()
	if len(allowed) == 0 {
		delete(h, headerBaggage)
		return
	}
	b, err := baggage.Parse(strings.Join(values, ","))
	if err != nil {
		delete(h, headerBaggage)
		return
	}
	var keep []baggage.Member
	for _, m := range b.Members() {
		if _, ok := allowed[m.Key()]; ok {
			// Key and value only: member properties are free-form and
			// could carry anything past the allowlist.
			bare, err := baggage.NewMemberRaw(m.Key(), m.Value())
			if err != nil {
				continue
			}
			keep = append(keep, bare)
		}
	}
	if len(keep) == 0 {
		delete(h, headerBaggage)
		return
	}
	kept, err := baggage.New(keep...)
	if err != nil {
		delete(h, headerBaggage)
		return
	}
	h[headerBaggage] = []string{kept.String()}
}

// dropTracestate removes tracestate, in any case spelling. A caller's
// tracestate is vendor-defined and opaque, so it is never forwarded
// upstream; traceparent carries the trace on its own.
func dropTracestate(h http.Header) {
	for k := range h {
		if strings.EqualFold(k, headerTracestate) {
			delete(h, k)
		}
	}
}

// FilterTraceContext applies the outbound rules to headers a caller
// supplied: baggage reduced to the allowlist, tracestate removed.
func FilterTraceContext(h http.Header) {
	FilterBaggage(h)
	dropTracestate(h)
}

// StripTraceContext removes traceparent, tracestate and baggage, for an
// upstream that opted out of trace propagation (a per-LLM setting).
func StripTraceContext(h http.Header) {
	delete(h, headerTraceparent)
	delete(h, headerTracestate)
	delete(h, headerBaggage)
}
