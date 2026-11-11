package grpc

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pulseRecords are the records SendAnalyticsPulse built from one pulse.
type pulseRecords struct {
	proxyLogs        []*models.ProxyLog
	chatRecords      []*models.LLMChatRecord
	complianceEvents []*models.ComplianceEvent
	toolCalls        []*models.ToolCallRecord
}

func (r *pulseRecords) empty() bool {
	return len(r.proxyLogs) == 0 && len(r.chatRecords) == 0 && len(r.complianceEvents) == 0 && len(r.toolCalls) == 0
}

// storePulseRecords stores the records of a pulse and reports whether the
// pulse was a repeat of one already stored (nothing is stored then).
//
// A pulse with a boot ID has an identity (edge ID, boot ID, sequence number)
// that stays the same when the edge sends it again. Such a pulse is stored
// in one transaction together with its key, and the edge is answered only
// once that has committed: if the answer is lost (this control plane goes
// away, the edge moves to another) the repeat finds the key and is
// acknowledged without being stored, and if the store fails the edge is told
// so and sends the batch again. Without this, a batch whose acknowledgement
// was lost was stored twice, and spend with it (features/ActiveActive.md,
// Phase 2 item 4).
//
// A pulse without a boot ID (an edge from before this) is recorded as it
// always was, through the background writer, and so is every pulse when the
// analytics handler in use cannot store a batch once (a host's own handler).
func (s *ControlServer) storePulseRecords(ctx context.Context, req *pb.AnalyticsPulse, r *pulseRecords) (repeated bool, err error) {
	if r.empty() {
		return false, nil
	}
	if req.BootId != "" {
		stored, err := analytics.RecordEdgeBatch(ctx, &analytics.EdgeBatch{
			Key:              models.EdgeAnalyticsBatchKey(req.EdgeId, req.BootId, req.SequenceNumber),
			EdgeID:           req.EdgeId,
			ProxyLogs:        r.proxyLogs,
			ChatRecords:      r.chatRecords,
			ComplianceEvents: r.complianceEvents,
			ToolCalls:        r.toolCalls,
		})
		switch {
		case err == nil && stored:
			return false, nil
		case err == nil:
			logger.Log.Info().
				Str("edge_id", req.EdgeId).
				Uint64("sequence_number", req.SequenceNumber).
				Uint32("total_records", req.TotalRecords).
				Msg("Analytics pulse received again; it was stored before, so it is acknowledged without being stored")
			return true, nil
		case !errors.Is(err, analytics.ErrEdgeBatchUnsupported):
			logger.Log.Warn().Err(err).
				Str("edge_id", req.EdgeId).
				Uint64("sequence_number", req.SequenceNumber).
				Msg("Analytics pulse not stored; the edge will send it again")
			return false, status.Error(codes.Unavailable, "analytics pulse not stored; retry")
		}
	}

	analytics.RecordProxyLogsBatch(ctx, r.proxyLogs)
	analytics.RecordChatRecordsBatch(ctx, r.chatRecords)
	analytics.RecordComplianceEvents(ctx, r.complianceEvents)
	for _, tc := range r.toolCalls {
		analytics.RecordToolCall(ctx, tc.Name, tc.TimeStamp, tc.ExecTime, tc.ToolID)
	}
	return false, nil
}
