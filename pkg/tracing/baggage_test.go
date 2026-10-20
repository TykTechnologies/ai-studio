package tracing

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
)

func withAllowlist(t *testing.T, keys ...string) {
	t.Helper()
	SetBaggageAllowlist(keys)
	t.Cleanup(func() { SetBaggageAllowlist([]string{"session.id"}) })
}

func TestFilterBaggage(t *testing.T) {
	withAllowlist(t, "session.id", "tenant")
	cases := []struct {
		name string
		in   []string
		want []string // nil: header removed
	}{
		{"no header", nil, nil},
		{"keeps allowlisted only", []string{"session.id=s1,user.email=a%40b.c,tenant=t9"}, []string{"session.id=s1,tenant=t9"}},
		{"multiple header lines", []string{"user.id=u1", "session.id=s2"}, []string{"session.id=s2"}},
		{"nothing allowed", []string{"user.id=u1,secret=x"}, nil},
		{"malformed is removed", []string{"=;;=,,"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.in != nil {
				h["Baggage"] = tc.in
			}
			FilterBaggage(h)
			got := h["Baggage"]
			if tc.want == nil {
				if got != nil {
					t.Fatalf("baggage kept: %v", got)
				}
				return
			}
			b1, _ := baggage.Parse(got[0])
			b2, _ := baggage.Parse(tc.want[0])
			if len(got) != 1 || b1.Len() != b2.Len() {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for _, m := range b2.Members() {
				if b1.Member(m.Key()).Value() != m.Value() {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestFilterBaggageNonCanonicalKey(t *testing.T) {
	withAllowlist(t, "session.id")
	h := http.Header{"baggage": {"session.id=s1,user.id=u"}}
	FilterBaggage(h)
	if len(h) != 1 || h.Get("Baggage") != "session.id=s1" {
		t.Fatalf("got %v", h)
	}
}

func TestFilterBaggageEmptyAllowlist(t *testing.T) {
	withAllowlist(t)
	h := http.Header{"Baggage": {"session.id=s1"}}
	FilterBaggage(h)
	if h["Baggage"] != nil {
		t.Fatal("an empty allowlist must forward no baggage")
	}
}

// InjectOutgoing filters baggage whether it came from the context (an
// extracted client header) or was already on the outgoing request.
func TestInjectOutgoingFiltersBaggage(t *testing.T) {
	withAllowlist(t, "session.id")
	Use(nil, propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { Use(nil, nil) })

	in := http.Header{
		"Traceparent": {"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		"Baggage":     {"session.id=s1,user.email=leak"},
	}
	ctx := ExtractIncoming(context.Background(), in)
	out := http.Header{"Baggage": {"user.id=also-leak"}}
	InjectOutgoing(ctx, out)
	if out.Get("Traceparent") == "" {
		t.Fatal("traceparent not propagated")
	}
	if got := out.Get("Baggage"); got != "session.id=s1" {
		t.Fatalf("baggage = %q", got)
	}
}

func TestStripTraceContext(t *testing.T) {
	h := http.Header{"Traceparent": {"x"}, "Tracestate": {"y"}, "Baggage": {"z"}, "Other": {"keep"}}
	StripTraceContext(h)
	if len(h) != 1 || h.Get("Other") != "keep" {
		t.Fatalf("got %v", h)
	}
}

func TestFilterBaggageNoHeaderAllocatesNothing(t *testing.T) {
	h := http.Header{"Content-Type": {"application/json"}}
	if n := testing.AllocsPerRun(100, func() { FilterBaggage(h) }); n != 0 {
		t.Fatalf("allocates %.0f with no baggage header", n)
	}
}
