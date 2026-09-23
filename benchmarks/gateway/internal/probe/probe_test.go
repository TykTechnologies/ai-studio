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
	for _, f := range []Format{FormatOpenAI, FormatAnthropic} {
		t.Run(string(f), func(t *testing.T) {
			srv := httptest.NewServer(mockllm.NewServer(mockllm.Profile{
				TTFT: 40 * time.Millisecond, TokensPerSecond: 100, OutputTokens: 6, TokenText: "a ",
			}))
			defer srv.Close()
			path := "/v1/chat/completions"
			if f == FormatAnthropic {
				path = "/v1/messages"
			}
			req := BodySpec{Format: f, Stream: true, Model: "m", MaxTokens: 6}.Build()
			res := Do(context.Background(), http.DefaultClient, Target{URL: srv.URL + path}, req, time.Now())

			if !res.OK() || res.Tokens != 6 {
				t.Fatalf("result: %+v", res)
			}
			if res.TTFT < 40 || res.TTFT > res.Total {
				t.Fatalf("ttft=%.1f total=%.1f", res.TTFT, res.Total)
			}
			// 5 more tokens at 10ms each.
			if res.Total-res.TTFT < 45 {
				t.Fatalf("stream body time %.1fms, want >= 45ms", res.Total-res.TTFT)
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
