package agenttrace

import (
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// ToProto converts an Event for the edge pulse and the staging table.
func ToProto(e *Event) *pb.CaptureEventProto {
	if e == nil {
		return nil
	}
	p := &pb.CaptureEventProto{
		Kind:           string(e.Kind),
		SpanId:         e.SpanID.Bytes(),
		TraceId:        e.TraceID.Bytes(),
		ParentSpanId:   e.ParentSpanID.Bytes(),
		Seq:            e.Seq,
		IdempotencyKey: e.IdempotencyKey,
		Source:         e.Source,
		Identity: &pb.CaptureIdentityProto{
			AppId:        uint64(e.Identity.AppID),
			TeamId:       uint64(e.Identity.TeamID),
			CredentialId: e.Identity.CredentialID,
			UserId:       uint64(e.Identity.UserID),
			EndUserId:    e.Identity.EndUserID,
			OnBehalfOf:   e.Identity.OnBehalfOf,
			ActingAgent:  e.Identity.ActingAgent,
		},
		Resource: &pb.CaptureResourceProto{
			Type: e.Resource.Type,
			Id:   e.Resource.ID,
			Name: e.Resource.Name,
		},
		Provider:      e.Provider,
		Operation:     e.Operation,
		RequestModel:  e.RequestModel,
		ResponseModel: e.ResponseModel,
		ToolName:      e.ToolName,
		StartUnixNano: unixNano(e.StartTime),
		EndUnixNano:   unixNano(e.EndTime),
		TtftNanos:     int64(e.TTFT),
		Status:        string(e.Status),
		ErrorType:     e.ErrorType,
		ErrorMessage:  e.ErrorMessage,
		HttpStatus:    int32(e.HTTPStatus),
		GatewayNode:   e.GatewayNode,
		Attempt:       e.Attempt,
		AttemptOf:     e.AttemptOf.Bytes(),
		Usage: &pb.CaptureUsageProto{
			InputTokens:      e.Usage.InputTokens,
			OutputTokens:     e.Usage.OutputTokens,
			CacheReadTokens:  e.Usage.CacheReadTokens,
			CacheWriteTokens: e.Usage.CacheWriteTokens,
			ReasoningTokens:  e.Usage.ReasoningTokens,
			CostMicros:       e.Usage.CostMicros,
			Currency:         e.Usage.Currency,
		},
		AnalyticsRef: e.AnalyticsRef,
		Declared:     declaredToProto(&e.Declared),
		Extracts:     extractsToProto(&e.Extracts),
		ContentMode:  e.ContentMode,
		Attributes:   copyMap(e.Attributes),
	}
	for _, ge := range e.GatewayEvents {
		p.GatewayEvents = append(p.GatewayEvents, &pb.CaptureGatewayEventProto{
			Type:       ge.Type,
			Outcome:    ge.Outcome,
			Name:       ge.Name,
			Detail:     ge.Detail,
			AtUnixNano: unixNano(ge.At),
		})
	}
	for _, part := range e.Parts {
		p.Parts = append(p.Parts, &pb.CapturePayloadPartProto{
			Part:           part.Part,
			ContentState:   part.ContentState,
			MediaType:      part.MediaType,
			Bytes:          part.Bytes,
			Truncated:      part.Truncated,
			RedactionCount: part.RedactionCount,
			Fingerprint:    part.Fingerprint,
			Data:           part.Data,
		})
	}
	return p
}

// FromProto converts a received event back. Malformed ids (wrong lengths)
// become zero ids; the assembler treats those as unset.
func FromProto(p *pb.CaptureEventProto) *Event {
	if p == nil {
		return nil
	}
	e := &Event{
		Kind:           Kind(p.Kind),
		SpanID:         spanIDFrom(p.SpanId),
		TraceID:        traceIDFrom(p.TraceId),
		ParentSpanID:   spanIDFrom(p.ParentSpanId),
		Seq:            p.Seq,
		IdempotencyKey: p.IdempotencyKey,
		Source:         p.Source,
		Provider:       p.Provider,
		Operation:      p.Operation,
		RequestModel:   p.RequestModel,
		ResponseModel:  p.ResponseModel,
		ToolName:       p.ToolName,
		StartTime:      fromUnixNano(p.StartUnixNano),
		EndTime:        fromUnixNano(p.EndUnixNano),
		TTFT:           time.Duration(p.TtftNanos),
		Status:         Status(p.Status),
		ErrorType:      p.ErrorType,
		ErrorMessage:   p.ErrorMessage,
		HTTPStatus:     int(p.HttpStatus),
		GatewayNode:    p.GatewayNode,
		Attempt:        p.Attempt,
		AttemptOf:      spanIDFrom(p.AttemptOf),
		AnalyticsRef:   p.AnalyticsRef,
		ContentMode:    p.ContentMode,
		Attributes:     boundAttributes(p.Attributes),
	}
	if id := p.Identity; id != nil {
		e.Identity = Identity{
			AppID:        uint(id.AppId),
			TeamID:       uint(id.TeamId),
			CredentialID: id.CredentialId,
			UserID:       uint(id.UserId),
			EndUserID:    id.EndUserId,
			OnBehalfOf:   id.OnBehalfOf,
			ActingAgent:  id.ActingAgent,
		}
	}
	if r := p.Resource; r != nil {
		e.Resource = Resource{Type: r.Type, ID: r.Id, Name: r.Name}
	}
	if u := p.Usage; u != nil {
		e.Usage = Usage{
			InputTokens:      u.InputTokens,
			OutputTokens:     u.OutputTokens,
			CacheReadTokens:  u.CacheReadTokens,
			CacheWriteTokens: u.CacheWriteTokens,
			ReasoningTokens:  u.ReasoningTokens,
			CostMicros:       u.CostMicros,
			Currency:         u.Currency,
		}
	}
	if d := p.Declared; d != nil {
		e.Declared = DeclaredContext{
			TraceID:         traceIDFrom(d.TraceId),
			ParentSpanID:    spanIDFrom(d.ParentSpanId),
			TraceState:      d.TraceState,
			Sampled:         d.Sampled,
			TraceSource:     d.TraceSource,
			SessionID:       d.SessionId,
			SessionSource:   d.SessionSource,
			SessionName:     d.SessionName,
			AgentName:       d.AgentName,
			AgentID:         d.AgentId,
			AgentPath:       d.AgentPath,
			ClientSpanID:    d.ClientSpanId,
			EndUserID:       d.EndUserId,
			CaptureMode:     d.CaptureMode,
			ExternalTraceID: d.ExternalTraceId,
			ConversationID:  d.ConversationId,
			MCPSessionID:    d.McpSessionId,
			ClientInfo:      d.ClientInfo,
			Baggage:         copyMap(d.Baggage),
			Harness:         d.Harness,
			TurnID:          d.TurnId,
			SubAgentID:      d.SubAgentId,
		}
	}
	if x := p.Extracts; x != nil {
		e.Extracts = Extracts{
			ResponseID:         x.ResponseId,
			PreviousResponseID: x.PreviousResponseId,
			FinishReasons:      append([]string(nil), x.FinishReasons...),
			OfferedTools:       append([]string(nil), x.OfferedTools...),
			ArgsFP:             fp(x.ArgsFp),
			ResultFP:           fp(x.ResultFp),
			ToolCallID:         x.ToolCallId,
			ToolError:          x.ToolError,
			Stream:             x.Stream,
			StreamIncomplete:   x.StreamIncomplete,
		}
		for _, tc := range x.ToolCalls {
			e.Extracts.ToolCalls = append(e.Extracts.ToolCalls, ToolCallRequest{
				ID: tc.Id, Name: tc.Name, ArgsFP: fp(tc.ArgsFp), ArgsUnavailable: tc.ArgsUnavailable,
			})
		}
		for _, tr := range x.ToolResults {
			e.Extracts.ToolResults = append(e.Extracts.ToolResults, ToolResult{
				ID: tr.Id, Name: tr.Name, CallArgsFP: fp(tr.CallArgsFp), ResultFP: fp(tr.ResultFp), IsError: tr.IsError,
			})
		}
		// An edge's lists are bounded again here, as the capture bounds its own.
		e.Extracts.Bound()
	}
	// An edge's gateway events are bounded like its attributes (jyx.171).
	for _, ge := range p.GatewayEvents {
		g := GatewayEvent{Type: ge.Type, Outcome: ge.Outcome, Name: ge.Name, Detail: ge.Detail, At: fromUnixNano(ge.AtUnixNano)}
		if len(e.GatewayEvents) == maxGatewayEvents || !boundGatewayEvent(&g) {
			e.gatewayEventsDropped++
			continue
		}
		e.GatewayEvents = append(e.GatewayEvents, g)
	}
	for _, part := range p.Parts {
		e.Parts = append(e.Parts, PayloadPart{
			Part:           part.Part,
			ContentState:   part.ContentState,
			MediaType:      part.MediaType,
			Bytes:          part.Bytes,
			Truncated:      part.Truncated,
			RedactionCount: part.RedactionCount,
			Fingerprint:    fp(part.Fingerprint),
			Data:           part.Data,
		})
	}
	// The capturing gateway ran the content pipeline before sending it.
	e.contentProcessed = true
	return e
}

func declaredToProto(d *DeclaredContext) *pb.CaptureDeclaredContextProto {
	return &pb.CaptureDeclaredContextProto{
		TraceId:         d.TraceID.Bytes(),
		ParentSpanId:    d.ParentSpanID.Bytes(),
		TraceState:      d.TraceState,
		Sampled:         d.Sampled,
		TraceSource:     d.TraceSource,
		SessionId:       d.SessionID,
		SessionSource:   d.SessionSource,
		SessionName:     d.SessionName,
		AgentName:       d.AgentName,
		AgentId:         d.AgentID,
		AgentPath:       d.AgentPath,
		ClientSpanId:    d.ClientSpanID,
		EndUserId:       d.EndUserID,
		CaptureMode:     d.CaptureMode,
		ExternalTraceId: d.ExternalTraceID,
		ConversationId:  d.ConversationID,
		McpSessionId:    d.MCPSessionID,
		ClientInfo:      d.ClientInfo,
		Baggage:         copyMap(d.Baggage),
		Harness:         d.Harness,
		TurnId:          d.TurnID,
		SubAgentId:      d.SubAgentID,
	}
}

func extractsToProto(x *Extracts) *pb.CaptureExtractsProto {
	p := &pb.CaptureExtractsProto{
		ResponseId:         x.ResponseID,
		PreviousResponseId: x.PreviousResponseID,
		FinishReasons:      append([]string(nil), x.FinishReasons...),
		OfferedTools:       append([]string(nil), x.OfferedTools...),
		ArgsFp:             x.ArgsFP,
		ResultFp:           x.ResultFP,
		ToolCallId:         x.ToolCallID,
		ToolError:          x.ToolError,
		Stream:             x.Stream,
		StreamIncomplete:   x.StreamIncomplete,
	}
	for _, tc := range x.ToolCalls {
		p.ToolCalls = append(p.ToolCalls, &pb.CaptureToolCallProto{
			Id: tc.ID, Name: tc.Name, ArgsFp: tc.ArgsFP, ArgsUnavailable: tc.ArgsUnavailable,
		})
	}
	for _, tr := range x.ToolResults {
		p.ToolResults = append(p.ToolResults, &pb.CaptureToolResultProto{
			Id: tr.ID, Name: tr.Name, CallArgsFp: tr.CallArgsFP, ResultFp: tr.ResultFP, IsError: tr.IsError,
		})
	}
	return p
}

func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromUnixNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

func fp(b []byte) Fingerprint {
	if len(b) == 0 {
		return nil
	}
	return Fingerprint(b)
}

func copyMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
