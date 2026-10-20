package grpc

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
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
		metrics.RecordTracingCaptureDropped(ctx, "edge_"+reason, int64(n))
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

// captureOnly reports whether a pulse carries nothing but capture data.
func captureOnly(req *pb.AnalyticsPulse) bool {
	return len(req.AnalyticsEvents) == 0 && len(req.BudgetEvents) == 0 && len(req.ProxySummaries) == 0 &&
		len(req.ComplianceEvents) == 0 && len(req.ToolCalls) == 0
}
