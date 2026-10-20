package proxy

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// maxEventDetail bounds a gateway event's detail. Details are short reasons
// (a block message, a skip reason), never request or response content.
const maxEventDetail = 256

// recordPolicyBlock counts a policy block (aistudio_policy_block_total) and,
// while agent tracing is on, records it on the request's capture as a
// gateway event (spec §3.2): budget denials as budget, request and response
// filters and tool filters as filter. detail is the block's reason.
func recordPolicyBlock(ctx context.Context, rule, blockType string, detail ...string) {
	metrics.RecordPolicyBlock(ctx, rule, blockType)
	rec := agenttrace.From(ctx)
	if rec == nil {
		return
	}
	typ := agenttrace.GatewayEventFilter
	if rule == "budget" {
		typ = agenttrace.GatewayEventBudget
	}
	ge := agenttrace.GatewayEvent{Type: typ, Outcome: agenttrace.OutcomeBlock, Name: rule}
	if len(detail) > 0 {
		ge.Detail = boundDetail(detail[0])
	}
	rec.AddGatewayEvent(ge)
	rec.Update(func(ev *agenttrace.Event) {
		ev.Status = agenttrace.StatusError
		if ev.ErrorType == "" {
			ev.ErrorType = "policy_" + typ
		}
	})
}

// recordGatewayEvent adds a gateway event to the request's capture.
func recordGatewayEvent(ctx context.Context, ge agenttrace.GatewayEvent) {
	rec := agenttrace.From(ctx)
	if rec == nil {
		return
	}
	ge.Detail = boundDetail(ge.Detail)
	rec.AddGatewayEvent(ge)
}

// captureApp stamps the authenticated App on the request's capture, so an
// event the request ends with (even a denial before any upstream call) is
// attributable to its App. Called where authentication puts the App on the
// context.
func captureApp(ctx context.Context, app *models.App) {
	rec := agenttrace.From(ctx)
	if rec == nil || app == nil {
		return
	}
	rec.Update(func(ev *agenttrace.Event) { fillAppIdentity(ev, app) })
}

// finishCapture completes a request's capture when no hook filled it: a
// request that ended in gateway events alone (a denial before the vendor
// call) becomes an errored span of its route's kind, and the status the
// client got is kept.
func finishCapture(rec *agenttrace.Recorder, r *http.Request, status int) {
	rec.Update(func(ev *agenttrace.Event) {
		if ev.HTTPStatus == 0 {
			ev.HTTPStatus = status
		}
		if status >= http.StatusBadRequest && ev.Status == "" {
			ev.Status = agenttrace.StatusError
		}
		if ev.Kind != "" || len(ev.GatewayEvents) == 0 {
			return
		}
		switch path := r.URL.Path; {
		case strings.HasPrefix(path, "/tools/"):
			ev.Kind = agenttrace.KindToolCall
		case strings.HasPrefix(path, "/datasource"):
			ev.Kind = agenttrace.KindDatasourceCall
		case strings.HasPrefix(path, "/plugins/"):
			ev.Kind = agenttrace.KindPluginEndpoint
		default:
			ev.Kind = agenttrace.KindLLMInference
		}
	})
}

// captureStatusWriter records the status a handler wrote (tracing on only).
type captureStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *captureStatusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureStatusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *captureStatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *captureStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// writtenStatus is the status a handler wrote to w so far, found through any
// wrapping writers; 0 when unknown (tracing off, or nothing written yet).
func writtenStatus(w http.ResponseWriter) int {
	for w != nil {
		if sw, ok := w.(*captureStatusWriter); ok {
			return sw.status
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return 0
		}
		w = u.Unwrap()
	}
	return 0
}

func boundDetail(s string) string {
	if len(s) <= maxEventDetail {
		return s
	}
	s = s[:maxEventDetail]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
