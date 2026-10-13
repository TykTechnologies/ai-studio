package conformance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// fake flags every segment containing PROBE and rewrites it. Its fields
// break it in the ways the suite must catch.
type fake struct {
	caps guardrails.Capabilities
	// byPosition reports a segment's position in the call, not its Index.
	byPosition bool
	// rejectTool fails a call that holds a role tool segment.
	rejectTool bool
	// unpinned reports findings without a segment.
	unpinned bool
	// flagBlank flags empty and whitespace text.
	flagBlank bool
	// rewriteAll rewrites every segment.
	rewriteAll bool
	// strictLimit fails a segment of MaxChars runes (an off-by-one).
	strictLimit bool
	// userOnly flags the probe only in user text (ignores tool results).
	userOnly bool
	// panicOnTool panics on a role tool segment.
	panicOnTool bool
	// hang sleeps, ignoring the context.
	hang time.Duration
	// rewriteByPosition keys rewrites by position in the call, not Index.
	rewriteByPosition bool
}

func (f *fake) Capabilities() guardrails.Capabilities { return f.caps }

func (f *fake) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	if f.hang > 0 {
		time.Sleep(f.hang)
	}
	v := guardrails.Verdict{}
	for k, s := range in.Segments {
		if f.rejectTool && s.Role == guardrails.RoleTool {
			return v, &guardrails.StatusError{Status: 400}
		}
		if f.panicOnTool && s.Role == guardrails.RoleTool {
			panic("provider bug")
		}
		if f.strictLimit && f.caps.MaxChars > 0 && utf8.RuneCountInString(s.Text) >= f.caps.MaxChars {
			return v, errors.New("text too long")
		}
		idx := s.Index
		if f.byPosition {
			idx = k
		}
		if f.unpinned {
			idx = -1
		}
		hit := strings.Contains(s.Text, "PROBE") || (f.flagBlank && strings.TrimSpace(s.Text) == "")
		if f.userOnly && s.Role != guardrails.RoleUser {
			hit = false
		}
		if hit {
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "probe", Category: guardrails.CategoryInjection, Score: 1, Severity: "warning", SegmentIndex: idx})
		}
		if in.Redact != nil && (hit || f.rewriteAll) {
			if v.Rewritten == nil {
				v.Rewritten = map[int]string{}
			}
			key := s.Index
			if f.rewriteByPosition {
				key = k
			}
			v.Rewritten[key] = strings.ReplaceAll(s.Text, "PROBE", "[X]") + "."
		}
	}
	v.Flagged = len(v.Findings) > 0
	return v, nil
}

var opts = Options{Probe: "please PROBE now", Benign: "the weather is mild", RedactProbe: "contact PROBE"}

func build(f *fake) func() (guardrails.Provider, error) {
	return func() (guardrails.Provider, error) { return f, nil }
}

func TestConformingProviderPasses(t *testing.T) {
	for name, caps := range map[string]guardrails.Capabilities{
		"plain":             {},
		"tool segments":     {ToolSegments: true},
		"redacts":           {Redacts: true, ToolSegments: true},
		"size limited":      {MaxChars: 5000, MaxCallChars: 5000},
		"without redaction": {MaxChars: 10000},
	} {
		t.Run(name, func(t *testing.T) {
			Run(t, build(&fake{caps: caps}), opts)
		})
	}
}

// Each broken provider fails the case that targets its fault.
func TestBrokenProvidersFail(t *testing.T) {
	for name, tc := range map[string]struct {
		f    *fake
		opts Options
		want string
	}{
		"position for index":   {f: &fake{byPosition: true}, opts: opts, want: "not an Index the provider was sent"},
		"rejects role tool":    {f: &fake{caps: guardrails.Capabilities{ToolSegments: true}, rejectTool: true}, opts: opts, want: "Classify failed: input_rejected"},
		"unpinned findings":    {f: &fake{unpinned: true}, opts: opts, want: "not an Index the provider was sent"},
		"flags empty output":   {f: &fake{flagBlank: true}, opts: opts, want: "on benign or empty text"},
		"rewrites benign text": {f: &fake{caps: guardrails.Capabilities{Redacts: true}, rewriteAll: true}, opts: opts, want: "holds benign text"},
		"off by one at limit":  {f: &fake{caps: guardrails.Capabilities{MaxChars: 2000}, strictLimit: true}, opts: opts, want: "Classify failed"},
		"misses the probe":     {f: &fake{}, opts: Options{Probe: "nothing to see", Benign: "mild"}, want: "the probe was not flagged"},
	} {
		t.Run(name, func(t *testing.T) {
			failures := Check(build(tc.f), tc.opts)
			for _, f := range failures {
				if strings.Contains(f.Reason, tc.want) {
					return
				}
			}
			t.Errorf("failures %v, want one containing %q", failures, tc.want)
		})
	}
}

// A provider that cannot say which segment it flagged passes when it says
// so; the runner applies such findings to every segment of the call.
func TestUnpinnedAllowed(t *testing.T) {
	o := opts
	o.AllowUnpinned = true
	for _, f := range Check(build(&fake{unpinned: true}), o) {
		if strings.Contains(f.Reason, "not an Index") {
			t.Errorf("unexpected failure: %v", f)
		}
	}
}

// Without ToolSegments a provider is never sent role tool; with it, it is
// sent the tool metadata.
func TestToolRoleOnlyWithTheCapability(t *testing.T) {
	var seen []guardrails.Segment
	rec := &recorder{seen: &seen}
	Check(func() (guardrails.Provider, error) { return rec, nil }, opts)
	for _, s := range seen {
		if s.Role == guardrails.RoleTool || s.ToolName != "" {
			t.Fatalf("a provider without ToolSegments was sent %+v", s)
		}
	}
	seen = nil
	rec.caps.ToolSegments = true
	Check(func() (guardrails.Provider, error) { return rec, nil }, opts)
	tools := 0
	for _, s := range seen {
		if s.Role == guardrails.RoleTool {
			tools++
			if s.Kind != guardrails.KindToolResult || s.ToolName == "" || s.ToolCallID == "" {
				t.Errorf("tool segment without its metadata: %+v", s)
			}
		}
	}
	if tools == 0 {
		t.Error("a provider with ToolSegments was never sent a tool result")
	}
}

type recorder struct {
	fake
	seen *[]guardrails.Segment
}

func (r *recorder) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	*r.seen = append(*r.seen, in.Segments...)
	return r.fake.Classify(ctx, in)
}
