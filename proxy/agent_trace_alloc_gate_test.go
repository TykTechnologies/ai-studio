//go:build agenttracebench

package proxy

// Deterministic part of the agent tracing perf gate (bead jyx.34): what the
// capture allocates per request does not depend on the machine, so CI can
// gate it where timings would flake. Tracing off allocates nothing on any
// hook; tracing on stays within budgets set at 1.5x the measured cost.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// Measured (jyx.149/.51): 1 KiB chat 91 allocs / 15 KB; 32 KiB agent-loop
// request (history plus tool results) 278 allocs / 57 KB.
var captureBudgets = []struct {
	name         string
	size         int
	agent        bool
	allocs       float64
	bytesPerCall uint64
}{
	{"1KiB chat", 1 << 10, false, 140, 24 << 10},
	{"32KiB chat", 32 << 10, false, 140, 24 << 10}, // the chat fast path does not parse the body
	{"32KiB agent loop", 32 << 10, true, 420, 90 << 10},
}

func benchRequestBody(size int, agent bool) []byte {
	pad := strings.Repeat("x", size)
	if agent {
		return []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"` + pad + `"},` +
			`{"role":"assistant","tool_calls":[{"id":"call_0","type":"function","function":{"name":"search","arguments":"{}"}}]},` +
			`{"role":"tool","tool_call_id":"call_0","content":"found"}]}`)
	}
	return []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"` + pad + `"}]}`)
}

func TestAgentTraceAllocGate(t *testing.T) {
	t.Run("tracing off allocates nothing", func(t *testing.T) {
		agenttrace.SetEnabled(false)
		p := &Proxy{}
		ctx := context.Background()
		r := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)
		resp := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}}
		req := mcp.CallToolRequest{}
		n := testing.AllocsPerRun(200, func() {
			r2, rec := beginRequestCapture(r)
			_ = r2
			rec.Finish()
			_ = withUpstreamContentType(r, resp)
			_, finish := beginMCPToolCapture(ctx, req)
			finish()
			p.captureTool(ctx, toolCapture{})
			_ = traceSpanIDFor(ctx)
		})
		if n != 0 {
			t.Fatalf("tracing off: %v allocs per request, want 0", n)
		}
	})

	agenttrace.SetEnabled(true)
	agenttrace.SetSink(discardSink{})
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 1, Key: []byte("proxy-agent-trace-test-key-32by")})
	agenttrace.SetContentProcessor(offModeProcessor{})
	t.Cleanup(func() {
		agenttrace.SetEnabled(false)
		agenttrace.SetSink(nil)
		agenttrace.SetFingerprinter(nil)
		agenttrace.SetContentProcessor(nil)
	})
	llm := &models.LLM{Name: "Primary", Vendor: models.OPENAI}
	app := &models.App{ID: 1}
	respBody := []byte(toolCallReply)
	for _, b := range captureBudgets {
		t.Run(b.name, func(t *testing.T) {
			body := benchRequestBody(b.size, b.agent)
			one := func() {
				r := httptest.NewRequest(http.MethodPost, "/llm/rest/primary/v1/chat/completions", nil)
				r.Header.Set("Traceparent", clientTraceparent)
				r, rec := beginRequestCapture(r)
				captureLLM(r, llm, app, http.StatusOK, nil, body, respBody, false)
				rec.Finish()
			}
			allocs := testing.AllocsPerRun(200, one)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			const runs = 200
			for i := 0; i < runs; i++ {
				one()
			}
			runtime.ReadMemStats(&after)
			perCall := (after.TotalAlloc - before.TotalAlloc) / runs
			t.Logf("%s: %.0f allocs, %d B per request (budget %.0f, %d B)", b.name, allocs, perCall, b.allocs, b.bytesPerCall)
			if allocs > b.allocs {
				t.Errorf("%s: %.0f allocs per request, budget %.0f", b.name, allocs, b.allocs)
			}
			if perCall > b.bytesPerCall {
				t.Errorf("%s: %d B per request, budget %d", b.name, perCall, b.bytesPerCall)
			}
		})
	}
}
