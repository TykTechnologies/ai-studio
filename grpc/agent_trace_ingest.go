package grpc

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ingestCaptureEvents stages an edge pulse's agent tracing capture events
// (decision D3: AnalyticsPulse field 15) and records the drops the edge
// reported. The write is synchronous so that a failure fails the pulse and
// the edge retries; the idempotency key (edge id + span id + seq) makes the
// retry safe. A pulse that also carries other records never fails over its
// capture events: those records have no idempotency key, so a retry would
// count them twice.
//
// With tracing off on the hub (Community Edition, or the start gate closed)
// the events are discarded and the pulse succeeds.
func (s *ControlServer) ingestCaptureEvents(ctx context.Context, req *pb.AnalyticsPulse) (uint64, error) {
	for reason, n := range req.CaptureDropped {
		metrics.RecordTracingCaptureDropped(ctx, edgeDropLabel(reason), int64(n))
	}
	if len(req.CaptureEvents) == 0 {
		return 0, nil
	}
	if !agenttrace.Enabled() {
		logger.Log.Debug().Str("edge_id", req.EdgeId).Int("capture_events", len(req.CaptureEvents)).
			Msg("Agent tracing is off on this hub; discarding edge capture events")
		return 0, nil
	}
	if req.EdgeId == "" {
		return 0, status.Error(codes.InvalidArgument, "capture events need an edge id")
	}
	events := make([]*agenttrace.Event, 0, len(req.CaptureEvents))
	for _, p := range req.CaptureEvents {
		if ev := agenttrace.FromProto(p); ev != nil && !ev.SpanID.IsZero() {
			events = append(events, ev)
		}
	}
	events, err := s.scopeCaptureEvents(ctx, req.EdgeId, events)
	if err != nil {
		return 0, err
	}
	inserted, err := analytics.StageCaptureEvents(s.db, req.EdgeId, events)
	if err != nil {
		logger.Log.Warn().Err(err).Str("edge_id", req.EdgeId).Int("capture_events", len(events)).
			Msg("Failed to stage edge capture events")
		if captureOnly(req) {
			return 0, status.Error(codes.Unavailable, "capture events not staged; retry")
		}
		metrics.RecordTracingCaptureDropped(ctx, agenttrace.DropWriteError, int64(len(events)))
		return 0, nil
	}
	logger.Log.Debug().Str("edge_id", req.EdgeId).Int("capture_events", len(events)).Int64("inserted", inserted).
		Msg("Edge capture events staged")
	return uint64(len(events)), nil
}

// edgeDropLabel maps an edge-reported drop reason to a fixed metric label,
// so an edge cannot choose Prometheus label values.
func edgeDropLabel(reason string) string {
	switch reason {
	case agenttrace.DropPayload, agenttrace.DropGatewayEventDetail, agenttrace.DropSpan,
		agenttrace.DropNoSealer, agenttrace.DropWriteError:
		return "edge_" + reason
	}
	return "edge_other"
}

// Capture drop reasons at ingest (jyx.67).
const (
	dropUnregisteredEdge = "edge_unregistered"
	dropAppOutOfScope    = "edge_app_out_of_scope"
)

// scopeCaptureEvents keeps the events an edge may report. Every edge shares
// one bearer token, so the pulse's own edge id and namespace prove nothing
// (jyx.67). The hub therefore looks the edge up in its registry and takes
// the namespace it recorded at registration. It keeps only events for Apps
// that namespace serves (the Apps the edge's snapshot carries) and events
// with no App. An edge id the hub never registered reports nothing.
func (s *ControlServer) scopeCaptureEvents(ctx context.Context, edgeID string, events []*agenttrace.Event) ([]*agenttrace.Event, error) {
	if len(events) == 0 {
		return events, nil
	}
	var edge models.EdgeInstance
	if err := edge.GetByEdgeID(s.db, edgeID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			metrics.RecordTracingCaptureDropped(ctx, dropUnregisteredEdge, int64(len(events)))
			logger.Log.Warn().Str("edge_id", edgeID).Int("capture_events", len(events)).
				Msg("Capture events from an unregistered edge id; discarded")
			return nil, nil
		}
		return nil, status.Error(codes.Unavailable, "edge registry unavailable; retry")
	}
	ids := map[uint]bool{}
	for _, ev := range events {
		if ev.Identity.AppID != 0 {
			ids[ev.Identity.AppID] = false
		}
	}
	if len(ids) > 0 {
		want := make([]uint, 0, len(ids))
		for id := range ids {
			want = append(want, id)
		}
		q := s.db.Model(&models.App{}).Where("id IN ?", want)
		if edge.Namespace == "" {
			q = q.Where("namespace = ''")
		} else {
			q = q.Where("(namespace = '' OR namespace = ?)", edge.Namespace)
		}
		var allowed []uint
		if err := q.Pluck("id", &allowed).Error; err != nil {
			return nil, status.Error(codes.Unavailable, "app lookup failed; retry")
		}
		for _, id := range allowed {
			ids[id] = true
		}
	}
	kept := events[:0]
	dropped := 0
	for _, ev := range events {
		if ev.Identity.AppID != 0 && !ids[ev.Identity.AppID] {
			dropped++
			continue
		}
		kept = append(kept, ev)
	}
	if dropped > 0 {
		metrics.RecordTracingCaptureDropped(ctx, dropAppOutOfScope, int64(dropped))
		logger.Log.Warn().Str("edge_id", edgeID).Str("namespace", edge.Namespace).Int("dropped", dropped).
			Msg("Capture events for Apps outside the edge's namespace; discarded")
	}
	return kept, nil
}

// maxEdgeRequestID is proxy_logs.request_id's column size.
const maxEdgeRequestID = 64

// edgeTraceSpanID is an edge proxy log's trace_span_id as the hub stores it:
// a non-zero 16-hex span id, lower-cased, or empty. An edge's value is
// untrusted, and an over-long one would fail the row's whole batch on
// Postgres.
func edgeTraceSpanID(s string) string {
	if s == "" {
		return ""
	}
	id, ok := agenttrace.ParseSpanID(s)
	if !ok {
		return ""
	}
	return id.String()
}

// edgeRequestID is an edge proxy log's request_id as the hub stores it: an
// edge mints request ids itself, so one over the column size (or with
// control characters) is not one, and is cleared.
func edgeRequestID(s string) string {
	if len(s) > maxEdgeRequestID {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return ""
		}
	}
	return s
}

// captureOnly reports whether a pulse carries nothing but capture data.
func captureOnly(req *pb.AnalyticsPulse) bool {
	return len(req.AnalyticsEvents) == 0 && len(req.BudgetEvents) == 0 && len(req.ProxySummaries) == 0 &&
		len(req.ComplianceEvents) == 0 && len(req.ToolCalls) == 0
}
