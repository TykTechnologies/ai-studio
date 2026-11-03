package scripting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Pieces know where they lie in their segment, seams included, in bytes,
// so a provider's span in a piece maps onto the segment
// (midsommar-clean-9u1j.65).
func TestSplitSegment_PiecesKnowWhereTheyAre(t *testing.T) {
	text := strings.Repeat("é→a ", 40) + "SECRET" + strings.Repeat(" ü", 30)
	for _, seams := range []bool{false, true} {
		ps := splitSegment(3, text, 25, seams)
		if len(ps) < 4 {
			t.Fatalf("pieces = %d", len(ps))
		}
		for _, p := range ps {
			if !p.sliced || p.parent != 3 || text[p.at:p.at+len(p.text)] != p.text {
				t.Fatalf("piece %+v is not the segment's bytes from %d", p, p.at)
			}
		}
	}
	if ps := splitSegment(0, text, 0, false); len(ps) != 1 || !ps[0].sliced || ps[0].at != 0 {
		t.Errorf("a whole segment %+v", ps)
	}
	// Invalid UTF-8 changes length in the pieces: none is a slice.
	bad := strings.Repeat("ab\xffcd ", 20)
	for _, p := range splitSegment(0, bad, 10, true) {
		if p.sliced {
			t.Fatalf("piece of invalid UTF-8 marked as a slice: %+v", p)
		}
	}
}

func TestPiece_SegmentSpan(t *testing.T) {
	p := piece{text: "0123456789", sliced: true, at: 100}
	for _, c := range []struct {
		in   *guardrails.Span
		want *guardrails.Span
	}{
		{nil, nil},
		{&guardrails.Span{Start: 2, End: 5}, &guardrails.Span{Start: 102, End: 105}},
		{&guardrails.Span{Start: 0, End: 10}, &guardrails.Span{Start: 100, End: 110}},
		{&guardrails.Span{Start: 3, End: 11}, nil}, // past the piece
		{&guardrails.Span{Start: -1, End: 4}, nil}, // before it
		{&guardrails.Span{Start: 6, End: 5}, nil},  // backwards
	} {
		got := p.segmentSpan(c.in)
		if (got == nil) != (c.want == nil) || got != nil && *got != *c.want {
			t.Errorf("segmentSpan(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := (piece{text: "0123456789", at: 100}).segmentSpan(&guardrails.Span{Start: 2, End: 5}); got != nil {
		t.Errorf("a piece that is no slice mapped a span: %v", got)
	}
}

// remoteSpanFake flags SECRET as a prompt attack, reporting where it is in the
// text it was sent (a piece) when withSpan is set, as a classifier such as
// Wolf Defender does once it reports the deciding window.
func remoteSpanFake(withSpan bool, caps guardrails.Capabilities) *fakeProvider {
	return &fakeProvider{caps: caps, verdict: func(in guardrails.Input) guardrails.Verdict {
		v := guardrails.Verdict{}
		for _, s := range in.Segments {
			at := strings.Index(s.Text, "SECRET")
			if at < 0 {
				continue
			}
			f := guardrails.Finding{Detector: "prompt_attack", Category: guardrails.CategoryInjection, Score: 1, Severity: "critical", SegmentIndex: s.Index}
			if withSpan {
				f.Span = &guardrails.Span{Start: max(at-200, 0), End: min(at+206, len(s.Text))}
			}
			v.Flagged = true
			v.Findings = append(v.Findings, f)
		}
		return v
	}}
}

func remoteSpanFilter(t *testing.T, p *fakeProvider) *models.Filter {
	t.Helper()
	cfg := installFake(t, fmt.Sprintf("fake_span_%d", judgeFakeSeq.Add(1)), p, guardrails.ActionBlock)
	cfg["judge"] = map[string]any{"mode": "soften", "llm_id": 70, "timeout_ms": 2000}
	f := guardrailFilter("wolf", false, cfg)
	f.ID = 13
	return f
}

// longBody is a user message of about 20k characters with SECRET in the
// middle: far longer than the judge's window.
func longBody() string {
	pad := strings.Repeat("ordinary review text, ", 450)
	return `{"messages":[{"role":"user","content":"` + pad + `SECRET` + pad + `"}]}`
}

// A remote classifier's span reaches the judge: on a segment longer than
// the judge's window, a benign opinion on the window around the span
// softens, and the window the judge saw holds what was flagged. Without a
// span the opinion is partial and the block stands.
func TestJudgeSoften_RemoteSpanLetsTheJudgeCoverALongSegment(t *testing.T) {
	for _, c := range []struct {
		name     string
		caps     guardrails.Capabilities
		withSpan bool
		softened bool
	}{
		{"whole segment, span", guardrails.Capabilities{}, true, true},
		{"whole segment, no span", guardrails.Capabilities{}, false, false},
		{"split into pieces, span", guardrails.Capabilities{MaxChars: 6000}, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			stub := installStubJudge(t, answering(`{"verdict":"benign","reason":"code_or_data"}`))
			withCapability(t, guardrails.CapabilityReadOnly)
			p := remoteSpanFake(c.withSpan, c.caps)
			out, err := NewFilterRunner(remoteSpanFilter(t, p)).RunScript(judgeInput(newEventSink(), longBody()), newJudgeSvc())
			if err != nil {
				t.Fatal(err)
			}
			if out.Block == c.softened {
				t.Fatalf("blocked = %v, want softened = %v", out.Block, c.softened)
			}
			if c.caps.MaxChars > 0 && len(p.seen()) < 3 {
				t.Fatalf("the segment was not split: %d pieces", len(p.seen()))
			}
			jes := judgeEvents(out.ComplianceEvents)
			if len(jes) != 1 {
				t.Fatalf("judge events %+v", jes)
			}
			want := "partial"
			if c.withSpan {
				want = "full"
			}
			if j := judgeMeta(t, jes[0]); j["coverage"] != want {
				t.Errorf("coverage = %v, want %s", j["coverage"], want)
			}
			if c.withSpan && (stub.calls.Load() != 1 || !strings.Contains(stub.users[0], "SECRET")) {
				t.Errorf("the judge was not shown the flagged text (calls %d)", stub.calls.Load())
			}
		})
	}
}
