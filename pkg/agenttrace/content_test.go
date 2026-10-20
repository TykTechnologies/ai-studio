package agenttrace

import (
	"bytes"
	"context"
	"strings"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/protobuf/proto"
)

const rawCanary = "raw-content-canary-7f3a"

func TestCaptureRaw_CopiesAndCaps(t *testing.T) {
	withEnabled(t, true)
	defer SetMaxRawPartBytes(4 << 20)
	SetMaxRawPartBytes(8)

	ctx, rec := Begin(context.Background())
	buf := []byte("0123456789abcdef")
	CaptureRaw(ctx, PartRequest, "application/json", buf)
	buf[0] = 'X'
	rec.Update(func(ev *Event) {
		if len(ev.Raw) != 1 {
			t.Fatalf("raw parts = %d", len(ev.Raw))
		}
		r := ev.Raw[0]
		if string(r.Data) != "01234567" || r.Bytes != 16 {
			t.Fatalf("raw = %q (%d bytes)", r.Data, r.Bytes)
		}
	})

	// No recorder (flag off, or nothing begun): nothing kept.
	CaptureRaw(context.Background(), PartRequest, "", buf)
}

type upperProcessor struct{ calls int }

func (u *upperProcessor) Process(ev *Event) {
	u.calls++
	ev.ContentMode = ContentFull
	for _, r := range ev.Raw {
		ev.Parts = append(ev.Parts, PayloadPart{Part: r.Part, ContentState: StateFull, Data: bytes.ToUpper(r.Data), Bytes: r.Bytes})
	}
}

// Without a processor raw content is dropped, never serialised.
func TestProcessContent_FailsClosedWithoutProcessor(t *testing.T) {
	SetContentProcessor(nil)
	ev := &Event{Raw: []RawPart{{Part: PartRequest, MediaType: "application/json", Data: []byte(rawCanary), Bytes: 99}},
		ErrorMessage: strings.Repeat("é", 300)}
	ProcessContent(ev)
	if ev.Raw != nil || len(ev.Parts) != 1 {
		t.Fatalf("raw=%v parts=%v", ev.Raw, ev.Parts)
	}
	p := ev.Parts[0]
	if p.ContentState != StateDropped || p.Data != nil || p.Bytes != 99 {
		t.Fatalf("part = %+v", p)
	}
	if len(ev.ErrorMessage) > maxUnprocessedError || !utf8Valid(ev.ErrorMessage) {
		t.Fatalf("error message not cut on a rune boundary: %d bytes", len(ev.ErrorMessage))
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }

func TestProcessContent_RunsOnce(t *testing.T) {
	p := &upperProcessor{}
	SetContentProcessor(p)
	t.Cleanup(func() { SetContentProcessor(nil) })

	ev := &Event{Raw: []RawPart{{Part: PartRequest, Data: []byte("abc"), Bytes: 3}}}
	ProcessContent(ev)
	ProcessContent(ev)
	if p.calls != 1 || ev.Raw != nil || len(ev.Parts) != 1 || string(ev.Parts[0].Data) != "ABC" {
		t.Fatalf("calls=%d raw=%v parts=%+v", p.calls, ev.Raw, ev.Parts)
	}

	// An event an edge processed (decoded from the pulse) is not processed
	// again at the hub.
	edge := FromProto(ToProto(&Event{Kind: KindLLMInference, ErrorMessage: "edge-redacted"}))
	ProcessContent(edge)
	if p.calls != 1 {
		t.Fatal("hub re-processed an edge event")
	}
}

// Staging never holds raw content: EncodeStaged runs the pipeline (or drops
// the content), and ToProto ignores raw parts.
func TestEncodeStaged_NeverCarriesRaw(t *testing.T) {
	SetSealer(nil)
	SetContentProcessor(nil)
	ev := &Event{Kind: KindLLMInference, IdempotencyKey: "embedded:01:0",
		Raw: []RawPart{{Part: PartRequest, Data: []byte(rawCanary), Bytes: int64(len(rawCanary))}}}
	blob, _, err := EncodeStaged(ev)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte(rawCanary)) {
		t.Fatal("raw content in the staged blob")
	}
	back, err := DecodeStaged(blob, ev.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Parts) != 1 || back.Parts[0].ContentState != StateDropped {
		t.Fatalf("parts = %+v", back.Parts)
	}

	unprocessed := &Event{Kind: KindLLMInference, Raw: []RawPart{{Part: PartRequest, Data: []byte(rawCanary)}}}
	wire, err := proto.Marshal(ToProto(unprocessed))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte(rawCanary)) {
		t.Fatal("ToProto serialised raw content")
	}
	var p pb.CaptureEventProto
	if err := proto.Unmarshal(wire, &p); err != nil || len(p.Parts) != 0 {
		t.Fatalf("parts on the wire: %v %v", p.Parts, err)
	}
}

func TestApproxSize_CountsRaw(t *testing.T) {
	small := (&Event{}).ApproxSize()
	big := (&Event{Raw: []RawPart{{Data: make([]byte, 10000)}}}).ApproxSize()
	if big-small < 10000 {
		t.Fatalf("raw bytes not counted: %d vs %d", big, small)
	}
}
