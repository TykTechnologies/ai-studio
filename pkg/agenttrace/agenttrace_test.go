package agenttrace

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/protobuf/proto"
)

func withEnabled(t testing.TB, on bool) {
	t.Helper()
	old := Enabled()
	SetEnabled(on)
	t.Cleanup(func() { SetEnabled(old) })
}

type captureSink struct {
	mu     sync.Mutex
	events []*Event
}

func (s *captureSink) Enqueue(ev *Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return true
}

func withSink(t testing.TB) *captureSink {
	t.Helper()
	s := &captureSink{}
	SetSink(s)
	t.Cleanup(func() { SetSink(nil) })
	return s
}

func fullEvent() *Event {
	ts := func(sec int64) time.Time { return time.Unix(1760000000+sec, 123456789).UTC() }
	f := StaticFingerprinter{ID: 7, Key: []byte("agenttrace-test-key-32-bytes-long")} // HMAC keys under 112 bits panic under fips140=only
	return &Event{
		Kind:           KindLLMInference,
		SpanID:         SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceID:        TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		ParentSpanID:   SpanID{9, 9, 9, 9, 9, 9, 9, 9},
		Seq:            3,
		IdempotencyKey: "edge-1:0102030405060708:3",
		Source:         "edge-1",
		Identity: Identity{
			AppID: 11, TeamID: 12, CredentialID: "cred", UserID: 13,
			EndUserID: "end-user", OnBehalfOf: "alice", ActingAgent: "bot",
		},
		Resource:      Resource{Type: ResourceLLM, ID: "4", Name: "gpt"},
		Provider:      "openai",
		Operation:     "chat",
		RequestModel:  "gpt-4o",
		ResponseModel: "gpt-4o-2024",
		ToolName:      "search",
		StartTime:     ts(0),
		EndTime:       ts(2),
		TTFT:          150 * time.Millisecond,
		Status:        StatusError,
		ErrorType:     "upstream_5xx",
		ErrorMessage:  "bad gateway",
		HTTPStatus:    502,
		GatewayNode:   "node-a",
		Attempt:       2,
		AttemptOf:     SpanID{7, 7, 7, 7, 7, 7, 7, 7},
		Usage: Usage{
			InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4,
			ReasoningTokens: 5, CostMicros: 6, Currency: "USD",
		},
		AnalyticsRef: "42",
		Declared: DeclaredContext{
			TraceID:         TraceID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
			ParentSpanID:    SpanID{8, 7, 6, 5, 4, 3, 2, 1},
			TraceState:      "vendor=x",
			Sampled:         true,
			TraceSource:     "traceparent",
			SessionID:       "sess-1",
			SessionSource:   "x-tyk-session-id",
			SessionName:     "triage",
			AgentName:       "researcher",
			AgentID:         "agent-1",
			AgentPath:       "/root/researcher",
			ClientSpanID:    "00f067aa0ba902b7",
			EndUserID:       "u-1",
			CaptureMode:     ContentRedacted,
			ExternalTraceID: "portkey-xyz",
			ConversationID:  "conv-1",
			MCPSessionID:    "mcp-1",
			ClientInfo:      "claude-code/2.0",
			Baggage:         map[string]string{"session.id": "sess-1"},
			Harness:         "codex_cli",
			TurnID:          "01a158d5-cbec-7402-855c-dc25520c80b0",
			SubAgentID:      "a42171bcd11f9f20c",
		},
		Extracts: Extracts{
			ToolCalls:          []ToolCallRequest{{ID: "call_1", Name: "search", ArgsFP: f.Fingerprint([]byte(`{}`)), ArgsUnavailable: true}},
			ToolResults:        []ToolResult{{ID: "call_0", ResultFP: f.Fingerprint([]byte(`"r"`)), IsError: true}},
			ResponseID:         "resp_1",
			PreviousResponseID: "resp_0",
			FinishReasons:      []string{"tool_calls"},
			OfferedTools:       []string{"search", "fetch"},
			ArgsFP:             f.Fingerprint([]byte(`{"q":1}`)),
			ResultFP:           f.Fingerprint([]byte(`"ok"`)),
			ToolCallID:         "call_2",
			ToolError:          true,
			Stream:             true,
			StreamIncomplete:   true,
		},
		GatewayEvents: []GatewayEvent{{
			Type: GatewayEventAttempt, Outcome: OutcomeError, Name: "gpt-4o", Detail: "attempt=1 status=503", At: ts(1),
		}},
		Parts: []PayloadPart{{
			Part: PartRequest, ContentState: StateTruncated, MediaType: "application/json", Bytes: 99,
			Truncated: true, RedactionCount: 2, Fingerprint: f.Fingerprint([]byte("x")), Data: []byte(`{"a":1}`),
		}},
		ContentMode: ContentRedacted,
		Attributes:  map[string]string{"mcp.method.name": "tools/call"},
	}
}

// notOnTheWire are Event fields that never cross a process boundary: raw
// content before the content pipeline, the pipeline's run-once marker, and
// the decoder's count of gateway events it dropped.
var notOnTheWire = map[string]bool{"Event.Raw": true, "Event.contentProcessed": true, "Event.gatewayEventsDropped": true}

// sameOnTheWire compares two events as the wire carries them.
func sameOnTheWire(a, b *Event) bool {
	ac, bc := *a, *b
	ac.Raw, bc.Raw = nil, nil
	ac.contentProcessed, bc.contentProcessed = false, false
	ac.gatewayEventsDropped, bc.gatewayEventsDropped = 0, 0
	return reflect.DeepEqual(&ac, &bc)
}

// assertAllSet fails for any zero-valued field, recursively, so a field added
// to the contract without a converter and a round-trip case fails this test.
func assertAllSet(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			if v.Interface().(time.Time).IsZero() {
				t.Errorf("%s is zero", path)
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			name := path + "." + v.Type().Field(i).Name
			if notOnTheWire[name] {
				continue
			}
			assertAllSet(t, v.Field(i), name)
		}
	case reflect.Slice:
		if v.Len() == 0 {
			t.Errorf("%s is empty", path)
			return
		}
		if v.Type().Elem().Kind() == reflect.Struct {
			assertAllSet(t, v.Index(0), path+"[0]")
		}
	default:
		if v.IsZero() {
			t.Errorf("%s is zero", path)
		}
	}
}

func TestProtoRoundTripCoversEveryField(t *testing.T) {
	in := fullEvent()
	assertAllSet(t, reflect.ValueOf(*in), "Event")

	wire, err := proto.Marshal(ToProto(in))
	if err != nil {
		t.Fatal(err)
	}
	var p pb.CaptureEventProto
	if err := proto.Unmarshal(wire, &p); err != nil {
		t.Fatal(err)
	}
	out := FromProto(&p)
	if !sameOnTheWire(in, out) {
		t.Fatalf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestProtoRoundTripZeroEvent(t *testing.T) {
	in := &Event{Kind: KindToolCall}
	wire, err := proto.Marshal(ToProto(in))
	if err != nil {
		t.Fatal(err)
	}
	var p pb.CaptureEventProto
	if err := proto.Unmarshal(wire, &p); err != nil {
		t.Fatal(err)
	}
	out := FromProto(&p)
	if !sameOnTheWire(in, out) {
		t.Fatalf("zero round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if FromProto(nil) != nil || ToProto(nil) != nil {
		t.Fatal("nil must convert to nil")
	}
}

func TestFromProtoMalformedIDs(t *testing.T) {
	out := FromProto(&pb.CaptureEventProto{SpanId: []byte{1, 2}, TraceId: []byte{1}})
	if !out.SpanID.IsZero() || !out.TraceID.IsZero() {
		t.Fatal("wrong-length ids must become zero ids")
	}
}

func TestIDs(t *testing.T) {
	s := NewSpanID()
	got, ok := ParseSpanID(s.String())
	if !ok || got != s {
		t.Fatal("span id hex round trip")
	}
	tr := NewTraceID()
	gt, ok := ParseTraceID(tr.String())
	if !ok || gt != tr {
		t.Fatal("trace id hex round trip")
	}
	for _, bad := range []string{"", "00000000000000000000000000000000", "zz", strings.Repeat("g", 32)} {
		if _, ok := ParseTraceID(bad); ok {
			t.Errorf("ParseTraceID(%q) accepted", bad)
		}
	}
	if _, ok := ParseSpanID("0000000000000000"); ok {
		t.Error("all-zero span id accepted")
	}
	if (SpanID{}).String() != "" || (SpanID{}).Bytes() != nil {
		t.Error("zero span id must render empty and nil")
	}
}

func TestIdempotencyKey(t *testing.T) {
	e := &Event{SpanID: SpanID{0, 0, 0, 0, 0, 0, 0xab, 0xcd}, Seq: 12}
	e.SetIdempotencyKey("edge-7")
	if e.IdempotencyKey != "edge-7:000000000000abcd:12" || e.Source != "edge-7" {
		t.Fatalf("got %q / %q", e.IdempotencyKey, e.Source)
	}
}

func TestStaticFingerprinter(t *testing.T) {
	f := StaticFingerprinter{ID: 3, Key: []byte("agenttrace-static-test-secret")}
	a, b := f.Fingerprint([]byte("x")), f.Fingerprint([]byte("x"))
	if a.String() != b.String() || a.KeyID() != 3 || len(a) != 33 {
		t.Fatalf("fingerprint not deterministic or wrong shape: %s", a)
	}
	if f.Fingerprint([]byte("y")).String() == a.String() {
		t.Fatal("different input, same fingerprint")
	}
	SetFingerprinter(f)
	t.Cleanup(func() { SetFingerprinter(nil) })
	if CurrentFingerprinter() == nil {
		t.Fatal("fingerprinter not installed")
	}
	SetFingerprinter(nil)
	if CurrentFingerprinter() != nil {
		t.Fatal("fingerprinter not removed")
	}
}

func TestRecorderOffIsInert(t *testing.T) {
	withEnabled(t, false)
	sink := withSink(t)
	ctx, r := Begin(context.Background())
	if r != nil || From(ctx) != nil {
		t.Fatal("recorder must be nil with the flag off")
	}
	AddPart(ctx, PayloadPart{Part: PartRequest})
	AddGatewayEvent(ctx, GatewayEvent{Type: GatewayEventBudget})
	Emit(ctx, &Event{Kind: KindGatewayEvent})
	r.Update(func(ev *Event) { t.Fatal("update ran on nil recorder") })
	r.Finish()
	r.Discard()
	if len(sink.events) != 0 {
		t.Fatal("events emitted with the flag off")
	}
}

func TestRecorderFinish(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	ctx, r := Begin(context.Background())
	if r == nil || r.SpanID().IsZero() {
		t.Fatal("recorder must exist with a span id")
	}
	if ctx2, r2 := Begin(ctx); r2 != r || ctx2 != ctx {
		t.Fatal("nested Begin must reuse the recorder")
	}
	r.Update(func(ev *Event) { ev.Kind = KindLLMInference; ev.Provider = "openai" })
	AddPart(ctx, PayloadPart{Part: PartRequest})
	AddGatewayEvent(ctx, GatewayEvent{Type: GatewayEventAttempt})
	Emit(ctx, &Event{Kind: KindGatewayEvent})
	r.Finish()
	r.Finish()
	AddPart(ctx, PayloadPart{Part: PartResponse}) // ignored after Finish

	if len(sink.events) != 2 {
		t.Fatalf("want 2 events, got %d", len(sink.events))
	}
	p := sink.events[0]
	if p.Provider != "openai" || p.Status != StatusOK || p.EndTime.IsZero() ||
		len(p.Parts) != 1 || len(p.GatewayEvents) != 1 || p.GatewayEvents[0].At.IsZero() {
		t.Fatalf("primary span not completed: %+v", p)
	}
	if x := sink.events[1]; x.SpanID.IsZero() || x.SpanID == p.SpanID || x.EndTime.IsZero() {
		t.Fatalf("extra event not completed: %+v", x)
	}
}

func TestRecorderWithoutKindEmitsOnlyExtras(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	ctx, r := Begin(context.Background())
	Emit(ctx, &Event{Kind: KindGatewayEvent})
	r.Finish()
	if len(sink.events) != 1 || sink.events[0].Kind != KindGatewayEvent {
		t.Fatalf("got %+v", sink.events)
	}
}

func TestRecorderDiscardAndNoSink(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	_, r := Begin(context.Background())
	r.Update(func(ev *Event) { ev.Kind = KindToolCall })
	r.Discard()
	r.Finish()
	if len(sink.events) != 0 {
		t.Fatal("discarded recorder emitted")
	}

	SetSink(nil)
	before := DroppedNoSink()
	_, r = Begin(context.Background())
	r.Update(func(ev *Event) { ev.Kind = KindToolCall })
	r.Finish()
	if DroppedNoSink() != before+1 {
		t.Fatal("no-sink drop not counted")
	}
}

func TestRecorderConcurrent(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	ctx, r := Begin(context.Background())
	r.Update(func(ev *Event) { ev.Kind = KindLLMInference })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				AddGatewayEvent(ctx, GatewayEvent{Type: GatewayEventAttempt})
				r.Update(func(ev *Event) { ev.Extracts.FinishReasons = append(ev.Extracts.FinishReasons, "stop") })
				_ = r.TraceID()
			}
		}()
	}
	done := make(chan struct{})
	go func() { r.Finish(); close(done) }()
	wg.Wait()
	r.Finish()
	<-done
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(sink.events))
	}
}

func TestOffPathAllocatesNothing(t *testing.T) {
	withEnabled(t, false)
	ctx := context.Background()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = Enabled()
		c, r := Begin(ctx)
		AddPart(c, PayloadPart{Part: PartRequest, Bytes: 10})
		AddGatewayEvent(c, GatewayEvent{Type: GatewayEventBudget, Outcome: OutcomeBlock, Detail: "over"})
		Emit(c, nil)
		r.Update(nil)
		_ = From(c).SpanID()
		r.Finish()
	})
	if allocs != 0 {
		t.Fatalf("off path allocates %.1f per run", allocs)
	}
}

func TestNoForbiddenDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, bad := range []string{
			"github.com/TykTechnologies/midsommar/v2/proxy",
			"github.com/TykTechnologies/midsommar/v2/models",
			"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io",
			"gorm" + ".io/", // split so scripts/gorm-verify.sh does not match the literal
		} {
			if dep == bad || strings.HasPrefix(dep, bad+"/") || strings.HasPrefix(dep, bad) && strings.HasSuffix(bad, "/") {
				t.Errorf("pkg/agenttrace depends on %s", dep)
			}
		}
		if strings.HasPrefix(dep, "golang.org/x/crypto") {
			t.Errorf("pkg/agenttrace depends on %s (FIPS rule: stdlib crypto only)", dep)
		}
	}
}

func BenchmarkEnabled(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Enabled()
	}
}

func BenchmarkRecorderOff(b *testing.B) {
	withEnabled(b, false)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, r := Begin(ctx)
		AddGatewayEvent(c, GatewayEvent{Type: GatewayEventBudget, Outcome: OutcomeBlock})
		AddPart(c, PayloadPart{Part: PartRequest})
		r.Finish()
	}
}

func BenchmarkRecorderOn(b *testing.B) {
	withEnabled(b, true)
	SetSink(nil)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, r := Begin(ctx)
		r.Update(func(ev *Event) { ev.Kind = KindLLMInference })
		AddGatewayEvent(c, GatewayEvent{Type: GatewayEventAttempt})
		r.Finish()
	}
}
