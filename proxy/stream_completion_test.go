package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneCompletion waits for the stream's OnStreamComplete and checks no second
// one follows.
func oneCompletion(t *testing.T, hook *streamCompleteHook) *StreamCompleteRequest {
	t.Helper()
	var got *StreamCompleteRequest
	select {
	case got = <-hook.done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnStreamComplete was not called")
	}
	select {
	case <-hook.done:
		t.Fatal("OnStreamComplete was called twice")
	case <-time.After(200 * time.Millisecond):
	}
	return got
}

// brokenStream sends the start of an event stream, then drops the
// connection: a transport error after the response began.
func brokenStream(contentType string, start []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(start)
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
}

// bpu: every streamed request gets exactly one OnStreamComplete, whichever
// way it ends, with a status no cache stores unless the stream completed:
// plugins release their in-flight state there (a load balancer's count, a
// rate limiter's slots, a cache's pending entry).
func TestStreamCompletion_EveryExitCompletesOnce(t *testing.T) {
	const path = "/llm/stream/primary/v1/chat/completions"
	const req = `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	cases := []struct {
		name    string
		serve   http.HandlerFunc
		filters []*models.Filter
		down    bool // the vendor cannot be reached
		status  int
	}{
		{name: "completed", serve: serveOpenAIText("hello there"), status: http.StatusOK},
		{name: "upstream error response", serve: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
		}, status: http.StatusTooManyRequests},
		{name: "filter block", serve: serveOpenAIText("FORBIDDEN text"), filters: []*models.Filter{forbiddenResponseGuardrail()},
			status: http.StatusBadRequest},
		{name: "tool call block", serve: dribble("text/event-stream", strings.ReplaceAll(callStreamCases()[0].body, "{ARG}", "EXFIL")),
			filters: []*models.Filter{streamCallGuardrail(true)}, status: http.StatusBadRequest},
		{name: "transport error", serve: serveOpenAIText("never"), down: true, status: http.StatusBadGateway},
		{name: "broken stream", serve: brokenStream("text/event-stream", []byte("data: "+openAIChunk("par", nil)+"\n\n")),
			status: http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, tc.serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Failover = models.LLMFailover{}
				primary.Filters = tc.filters
			})
			hook := &streamCompleteHook{done: make(chan *StreamCompleteRequest, 4)}
			h.proxy.AddResponseHook(hook)
			if tc.down {
				h.primaryVendor.server.Close()
			}

			_, _ = h.post(path, req)
			got := oneCompletion(t, hook)
			assert.Equal(t, tc.status, got.StatusCode)
			assert.JSONEq(t, req, string(got.RequestBody))
		})
	}
}

// The same on the native Bedrock stream.
func TestBedrockNativeStream_EveryExitCompletesOnce(t *testing.T) {
	const path = "/llm/stream/primary/model/m/converse-stream"
	const req = `{"messages":[{"role":"user","content":[{"text":"tidy my inbox"}]}]}`
	cases := []struct {
		name    string
		rec     *bedrockRecorder
		serve   http.HandlerFunc
		filters []*models.Filter
		down    bool
		status  int
	}{
		{name: "completed", rec: &bedrockRecorder{}, status: http.StatusOK},
		{name: "upstream error response", rec: &bedrockRecorder{status: http.StatusTooManyRequests, reply: []byte(`{"message":"slow down"}`)},
			status: http.StatusTooManyRequests},
		{name: "filter block", rec: &bedrockRecorder{}, filters: []*models.Filter{forbiddenResponseGuardrail()}, status: http.StatusBadRequest},
		{name: "tool call block", rec: &bedrockRecorder{}, filters: []*models.Filter{streamCallGuardrail(true)}, status: http.StatusBadRequest},
		{name: "transport error", rec: &bedrockRecorder{}, down: true, status: http.StatusBadGateway},
		{name: "broken stream", status: http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, args := "Sure. ", `{"to":"x"}`
			switch tc.name {
			case "filter block":
				text = "FORBIDDEN text"
			case "tool call block":
				args = `{"to":"EXFIL"}`
			}
			rec := tc.rec
			if rec == nil {
				rec = &bedrockRecorder{}
			}
			if rec.reply == nil {
				rec.reply = bedrockStream(t, text, args)
			}
			serve := rec.serve
			if tc.name == "broken stream" {
				serve = brokenStream("application/vnd.amazon.eventstream", rec.reply[:len(rec.reply)/2])
			}
			h := newFailoverHarness(t, serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Vendor = models.BEDROCK
				primary.Failover = models.LLMFailover{}
				primary.DefaultModel = "anthropic.claude-3"
				primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
				primary.Filters = tc.filters
			})
			hook := &streamCompleteHook{done: make(chan *StreamCompleteRequest, 4)}
			h.proxy.AddResponseHook(hook)
			if tc.down {
				h.primaryVendor.server.Close()
			}

			resp, body := h.postBedrock(path, req)
			got := oneCompletion(t, hook)
			assert.Equal(t, tc.status, got.StatusCode, "client got %d: %s", resp.StatusCode, body)
			require.JSONEq(t, req, string(got.RequestBody))
		})
	}
}
