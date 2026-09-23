package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/mockllm"
)

func TestDoStreamMeasuresFirstTokenSeparately(t *testing.T) {
	const (
		ttft   = 40 * time.Millisecond
		tps    = 100
		tokens = 6
		// The mock sends token i at ttft + i/tps, so the last token leaves
		// (tokens-1)/tps after the first.
		streamTime = time.Duration(tokens-1) * time.Second / tps
		// The client times the first token when it arrives and the end when
		// the last arrives; delivery of the two can differ by a scheduler
		// tick, so allow that much below the server-side spacing.
		deliverySlack = 5 * time.Millisecond
	)
	msf := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	for _, f := range []Format{FormatOpenAI, FormatAnthropic} {
		t.Run(string(f), func(t *testing.T) {
			srv := httptest.NewServer(mockllm.NewServer(mockllm.Profile{
				TTFT: ttft, TokensPerSecond: tps, OutputTokens: tokens, TokenText: "a ",
			}))
			defer srv.Close()
			path := "/v1/chat/completions"
			if f == FormatAnthropic {
				path = "/v1/messages"
			}
			req := BodySpec{Format: f, Stream: true, Model: "m", MaxTokens: tokens}.Build()
			res := Do(context.Background(), http.DefaultClient, Target{URL: srv.URL + path}, req, time.Now())

			if !res.OK() || res.Tokens != tokens {
				t.Fatalf("result: %+v", res)
			}
			if res.TTFT < msf(ttft) || res.TTFT > res.Total {
				t.Fatalf("ttft=%.1f total=%.1f, want ttft >= %.0fms", res.TTFT, res.Total, msf(ttft))
			}
			if min := msf(streamTime - deliverySlack); res.Total-res.TTFT < min {
				t.Fatalf("stream body time %.1fms, want >= %.0fms", res.Total-res.TTFT, min)
			}
		})
	}
}

func TestDoChargesLateReleaseToLatency(t *testing.T) {
	srv := httptest.NewServer(mockllm.NewServer(mockllm.Profile{OutputTokens: 1}))
	defer srv.Close()
	intended := time.Now().Add(-100 * time.Millisecond)
	res := Do(context.Background(), http.DefaultClient, Target{URL: srv.URL + "/v1/chat/completions"},
		BodySpec{Format: FormatOpenAI, Model: "m"}.Build(), intended)
	if res.Lag < 100 || res.Total < 100 {
		t.Fatalf("lag=%.1f total=%.1f, both should include the 100ms late release", res.Lag, res.Total)
	}
}

func TestDoReadsServerTimingTrailer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server-Timing", `gw-pre;dur=1.0`)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n"))
		w.(http.Flusher).Flush()
		w.Header().Set(http.TrailerPrefix+"Server-Timing", `gw-pre;dur=1.0, upstream;dur=9.5, gw;dur=2.25, conn;desc="reused"`)
	}))
	defer srv.Close()
	res := Do(context.Background(), http.DefaultClient, Target{URL: srv.URL},
		BodySpec{Format: FormatOpenAI, Stream: true}.Build(), time.Now())
	if res.Timing["upstream"] != 9.5 || res.Timing["gw"] != 2.25 || res.ConnGw != "reused" {
		t.Fatalf("timing=%v conn=%q", res.Timing, res.ConnGw)
	}
}

func TestBuildExtraOverridesAndDeletes(t *testing.T) {
	req := BodySpec{Format: FormatOpenAI, Model: "m", MaxTokens: 10, Extra: map[string]any{
		"max_tokens": nil, "temperature": nil, "max_completion_tokens": 10,
	}}.Build()
	got := string(req.Body)
	for _, bad := range []string{`"max_tokens"`, `"temperature"`} {
		if strings.Contains(got, bad) {
			t.Fatalf("%s should be deleted: %s", bad, got)
		}
	}
	if !strings.Contains(got, `"max_completion_tokens":10`) {
		t.Fatalf("extra not merged: %s", got)
	}
}

func TestFillerSize(t *testing.T) {
	if n := len(filler(32 * 1024)); n != 32*1024 {
		t.Fatalf("filler length %d", n)
	}
}
