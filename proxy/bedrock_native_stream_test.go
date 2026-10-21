package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
)

// bedrockFrame encodes one Converse stream event as AWS sends it.
func bedrockFrame(t *testing.T, eventType string, payload any) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, eventstream.NewEncoder().Encode(&out, eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.EventMessageType)},
			{Name: eventstreamapi.EventTypeHeader, Value: eventstream.StringValue(eventType)},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: b,
	}))
	return out.Bytes()
}

// bedrockStream is a Converse stream: text "Sure. ", then a toolUse whose
// input is args, then usage.
func bedrockStream(t *testing.T, text, args string) []byte {
	var s []byte
	s = append(s, bedrockFrame(t, "messageStart", map[string]any{"role": "assistant"})...)
	s = append(s, bedrockFrame(t, "contentBlockDelta", map[string]any{"contentBlockIndex": 0, "delta": map[string]any{"text": text}})...)
	s = append(s, bedrockFrame(t, "contentBlockStop", map[string]any{"contentBlockIndex": 0})...)
	s = append(s, bedrockFrame(t, "contentBlockStart", map[string]any{"contentBlockIndex": 1, "start": map[string]any{"toolUse": map[string]any{"toolUseId": "tu1", "name": "send"}}})...)
	s = append(s, bedrockFrame(t, "contentBlockDelta", map[string]any{"contentBlockIndex": 1, "delta": map[string]any{"toolUse": map[string]any{"input": args}}})...)
	s = append(s, bedrockFrame(t, "contentBlockStop", map[string]any{"contentBlockIndex": 1})...)
	s = append(s, bedrockFrame(t, "messageStop", map[string]any{"stopReason": "tool_use"})...)
	s = append(s, bedrockFrame(t, "metadata", map[string]any{"usage": map[string]any{"inputTokens": 10, "outputTokens": 5, "totalTokens": 15}})...)
	return s
}

// A rich Converse request: tools, a toolUse / toolResult history, an image
// and a document, and fields the gateway has never heard of.
const richConverse = `{"messages":[` +
	`{"role":"user","content":[{"text":"look at these"},{"image":{"format":"png","source":{"bytes":"iVBORw0KGgo="}}},{"document":{"format":"txt","name":"notes","source":{"bytes":"aGVsbG8="}}}]},` +
	`{"role":"assistant","content":[{"toolUse":{"toolUseId":"tu0","name":"read","input":{"path":"a.txt"}}}]},` +
	`{"role":"user","content":[{"toolResult":{"toolUseId":"tu0","content":[{"json":{"ok":true}}],"status":"success"}}]}],` +
	`"toolConfig":{"tools":[{"toolSpec":{"name":"read","description":"Read a file","inputSchema":{"json":{"type":"object"}}}}],"toolChoice":{"auto":{}}},` +
	`"additionalModelRequestFields":{"thinking":{"type":"enabled","budget_tokens":1024}},"inferenceConfig":{"maxTokens":64}}`

type bedrockRecorder struct {
	mu     sync.Mutex
	path   string
	body   []byte
	auth   string
	reply  []byte
	status int // answer with this status and reply as JSON, when set
}

func (b *bedrockRecorder) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	b.path, b.body, b.auth = r.URL.Path, body, r.Header.Get("Authorization")
	reply, status := b.reply, b.status
	b.mu.Unlock()
	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(reply)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
	for i := 0; i < len(reply); i += 13 {
		w.Write(reply[i:min(i+13, len(reply))])
		w.(http.Flusher).Flush()
		time.Sleep(time.Millisecond)
	}
}

func bedrockHarness(t *testing.T, rec *bedrockRecorder, filters ...*models.Filter) *failoverHarness {
	h := newFailoverHarness(t, rec.serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = "anthropic.claude-3"
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
		primary.Filters = filters
	})
	// The SDK client cache is process-wide and keyed by LLM ID, which every
	// test database starts again from 1: never hand this test a client (and
	// its pooled connections) built for an earlier test's fake Bedrock.
	bedrockVendor.InvalidateClientCache(h.primary.ID)
	t.Cleanup(func() { bedrockVendor.InvalidateClientCache(h.primary.ID) })
	return h
}

func (h *failoverHarness) postBedrock(path, body string) (*http.Response, []byte) {
	return h.post(path, body, "Authorization", "AWS4-HMAC-SHA256 Credential="+h.apiKey+"/20261019/us-east-1/bedrock/aws4_request, SignedHeaders=host, Signature=x")
}

// decodeFrames reads a binary event stream into (type, payload) pairs.
func decodeFrames(t *testing.T, b []byte) [][2]string {
	t.Helper()
	var out [][2]string
	r := bytes.NewReader(b)
	for r.Len() > 0 {
		msg, err := eventstream.NewDecoder().Decode(r, nil)
		require.NoError(t, err)
		name := ""
		if h := msg.Headers.Get(":event-type"); h != nil {
			name = h.String()
		}
		if h := msg.Headers.Get(":exception-type"); h != nil {
			name = "exception:" + h.String()
		}
		out = append(out, [2]string{name, string(msg.Payload)})
	}
	return out
}

// The native stream forwards the request as the client wrote it (tools,
// tool history, image, document, unknown fields), signed, and relays the
// vendor's frames byte for byte.
func TestBedrockNativeStream_Lossless(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, rec.reply, body, "frames relayed byte for byte")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Equal(t, "/model/anthropic.claude-3/converse-stream", rec.path)
	assert.JSONEq(t, richConverse, string(rec.body))
	assert.True(t, strings.HasPrefix(rec.auth, "AWS4-HMAC-SHA256 Credential=AKIDTEST/"), rec.auth)
}

// streamCompleteHook is a headerHook that also hands over what
// OnStreamComplete gets.
type streamCompleteHook struct {
	headerHook
	done chan *StreamCompleteRequest
}

func (h *streamCompleteHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	h.done <- req
	return &StreamCompleteResponse{}, nil
}

// Response hooks run on the native Bedrock stream as on every other stream
// (eac.83): the header phase sets headers (never the framing ones), the
// frames still go out byte for byte, and OnStreamComplete gets them with
// the client's request body. A vendor error gets the header phase too.
func TestBedrockNativeStream_RunsResponseHooks(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	hook := &streamCompleteHook{
		headerHook: headerHook{set: map[string]string{"X-Tyk-Compression": "applied", "Content-Type": "text/plain"}},
		done:       make(chan *StreamCompleteRequest, 1),
	}
	h.proxy.AddResponseHook(hook)

	resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, rec.reply, body, "frames relayed byte for byte")
	assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))
	assert.Equal(t, "application/vnd.amazon.eventstream", resp.Header.Get("Content-Type"))
	select {
	case got := <-hook.done:
		assert.Equal(t, rec.reply, got.AccumulatedResponse)
		assert.JSONEq(t, richConverse, string(got.RequestBody))
		assert.Equal(t, http.StatusOK, got.StatusCode)
	case <-time.After(5 * time.Second):
		t.Fatal("OnStreamComplete was not called")
	}

	rec.mu.Lock()
	rec.status, rec.reply = http.StatusBadRequest, []byte(`{"message":"bad request"}`)
	rec.mu.Unlock()
	resp, body = h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.JSONEq(t, `{"message":"bad request"}`, string(body))
	assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))
	select {
	case got := <-hook.done:
		assert.Equal(t, http.StatusBadRequest, got.StatusCode)
		assert.JSONEq(t, `{"message":"bad request"}`, string(got.AccumulatedResponse))
	case <-time.After(5 * time.Second):
		t.Fatal("OnStreamComplete was not called for the vendor error")
	}
}

// A plugin that holds per-request state until the stream completes (a load
// balancer's or rate limiter's in-flight count, a cache's pending entry)
// gets the completion for every native Bedrock stream, with its usage, and
// for a vendor error, so its count returns to zero (04k).
func TestBedrockNativeStream_CompletionReleasesInFlightState(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "hi", "{}")}
	h := bedrockHarness(t, rec)
	hook := &streamCompleteHook{done: make(chan *StreamCompleteRequest, 16)}
	h.proxy.AddResponseHook(hook)

	inFlight := 0 // what a post_auth hook would have counted
	for i := 0; i < 5; i++ {
		inFlight++
		resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	}
	rec.mu.Lock()
	rec.status, rec.reply = http.StatusTooManyRequests, []byte(`{"message":"throttled"}`)
	rec.mu.Unlock()
	inFlight++
	resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))

	completed := map[int]int{}
	for inFlight > 0 {
		select {
		case got := <-hook.done:
			inFlight--
			completed[got.StatusCode]++
			if got.StatusCode == http.StatusOK {
				assert.Contains(t, string(got.AccumulatedResponse), `"inputTokens":10`, "the usage reaches the hook")
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%d streams never completed", inFlight)
		}
	}
	assert.Equal(t, map[int]int{http.StatusOK: 5, http.StatusTooManyRequests: 1}, completed)
}

// The body's modelId (not a Converse field) names the model and is not
// forwarded; a non-streaming operation on the streaming route is refused.
func TestBedrockNativeStream_ModelIDAndOperations(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "hi", "{}")}
	h := bedrockHarness(t, rec)
	resp, body := h.postBedrock("/llm/stream/primary/converse-stream", `{"modelId":"m.from-body","messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	rec.mu.Lock()
	assert.Equal(t, "/model/m.from-body/converse-stream", rec.path)
	assert.NotContains(t, string(rec.body), "modelId")
	rec.mu.Unlock()

	resp, _ = h.postBedrock("/llm/stream/primary/model/m/converse", `{"messages":[]}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Request guardrails see the streamed request's tool results, as on the
// buffered path, and the quarantine reaches the vendor.
func TestBedrockNativeStream_ToolResultQuarantined(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "ok", "{}")}
	f := redactGuardrail("all_user")
	f.Config["on_detect"] = "block"
	f.Config["quarantine"] = map[string]any{"placeholder": "[gone]"}
	h := bedrockHarness(t, rec, f)
	body := `{"messages":[{"role":"user","content":[{"text":"go"}]},{"role":"assistant","content":[{"toolUse":{"toolUseId":"t","name":"read","input":{}}}]},{"role":"user","content":[{"toolResult":{"toolUseId":"t","content":[{"text":"API_KEY=SECRET"}]}}]}],"toolConfig":{"tools":[{"toolSpec":{"name":"read","inputSchema":{"json":{}}}}]}}`
	resp, out := h.postBedrock("/llm/stream/primary/model/m/converse-stream", body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.NotContains(t, string(rec.body), "SECRET")
	assert.Contains(t, string(rec.body), "[gone]")
	assert.Contains(t, string(rec.body), `"toolConfig"`)
}

// Response guardrails see the stream's text per chunk and in a final pass,
// and hold its toolUse; a block ends with a validationException frame.
func TestBedrockNativeStream_ResponseGuardrails(t *testing.T) {
	path := "/llm/stream/primary/model/m/converse-stream"
	req := `{"messages":[{"role":"user","content":[{"text":"tidy my inbox"}]}]}`

	rec := &bedrockRecorder{reply: bedrockStream(t, "FORBIDDEN text", `{"to":"x"}`)}
	_, body := bedrockHarness(t, rec, forbiddenResponseGuardrail()).postBedrock(path, req)
	frames := decodeFrames(t, body)
	require.NotEmpty(t, frames)
	assert.Equal(t, "exception:validationException", frames[len(frames)-1][0])
	assert.NotContains(t, string(body), "FORBIDDEN")

	rec = &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"EXFIL"}`)}
	_, body = bedrockHarness(t, rec, streamCallGuardrail(true)).postBedrock(path, req)
	frames = decodeFrames(t, body)
	assert.Contains(t, string(body), "Sure. ")
	assert.NotContains(t, string(body), "EXFIL")
	assert.NotContains(t, string(body), `"send"`)
	assert.Equal(t, "exception:validationException", frames[len(frames)-1][0])

	rec = &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	_, body = bedrockHarness(t, rec, streamCallGuardrail(true)).postBedrock(path, req)
	assert.Equal(t, rec.reply, body, "a clean call is released byte for byte")
}
