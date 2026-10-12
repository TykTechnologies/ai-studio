package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

// --- server-sent event rewriter ----------------------------------------------

func upperData(data []byte) ([]byte, error) {
	if bytes.Contains(data, []byte("fail")) {
		return nil, errors.New("boom")
	}
	return bytes.ToUpper(data), nil
}

func readAllSSE(t *testing.T, src io.Reader, rewrite func([]byte) ([]byte, error)) (string, error) {
	t.Helper()
	rw := newSSEEventRewriter(io.NopCloser(src), 1<<10, rewrite)
	out, err := io.ReadAll(rw)
	return string(out), err
}

func TestSSEEventRewriter(t *testing.T) {
	stream := "event: a\ndata: x1\n\n: comment\n\nevent: b\r\ndata: y2\r\n\r\ndata: z3"
	want := "event: a\ndata: X1\n\n: comment\n\nevent: b\ndata: Y2\n\ndata: Z3"
	// However the stream is chunked, every event is rewritten whole.
	for name, src := range map[string]io.Reader{
		"whole":        strings.NewReader(stream),
		"byte by byte": iotest.OneByteReader(strings.NewReader(stream)),
		"half reads":   iotest.HalfReader(strings.NewReader(stream)),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := readAllSSE(t, src, upperData)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	t.Run("unchanged events stay byte for byte", func(t *testing.T) {
		s := "event: a\r\ndata: same\r\n\r\nid: 7\ndata: same\n\n"
		got, err := readAllSSE(t, strings.NewReader(s), func(b []byte) ([]byte, error) { return b, nil })
		require.NoError(t, err)
		assert.Equal(t, s, got)
	})

	t.Run("multi-line data is one payload", func(t *testing.T) {
		var seen []string
		got, err := readAllSSE(t, strings.NewReader("event: e\ndata: a\ndata: b\nid: 1\n\n"), func(b []byte) ([]byte, error) {
			seen = append(seen, string(b))
			return []byte("ab"), nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"a\nb"}, seen)
		assert.Equal(t, "event: e\ndata: ab\nid: 1\n\n", got)
	})

	t.Run("each event is emitted before the next is read", func(t *testing.T) {
		pr, pw := io.Pipe()
		rw := newSSEEventRewriter(pr, 1<<10, upperData)
		go func() { _, _ = pw.Write([]byte("data: one\n\n")) }()
		buf := make([]byte, 64)
		n, err := rw.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "data: ONE\n\n", string(buf[:n]))
		_ = pw.Close()
	})

	t.Run("oversized event ends the stream", func(t *testing.T) {
		_, err := readAllSSE(t, strings.NewReader("data: "+strings.Repeat("x", 2<<10)+"\n\n"), upperData)
		assert.ErrorIs(t, err, errSSEEventTooLarge)
	})

	t.Run("a rewrite failure ends the stream", func(t *testing.T) {
		got, err := readAllSSE(t, strings.NewReader("data: ok\n\ndata: fail\n\ndata: never\n\n"), upperData)
		assert.Error(t, err)
		assert.Equal(t, "data: OK\n\n", got)
	})

	t.Run("a rewrite failure sends the error event, then ends", func(t *testing.T) {
		rw := newSSEEventRewriter(io.NopCloser(strings.NewReader("data: ok\n\ndata: fail\n\ndata: never\n\n")), 1<<10, upperData)
		rw.errorEvent = func(err error) []byte { return []byte(`{"error":"` + err.Error() + `"}`) }
		got, err := io.ReadAll(rw)
		assert.Error(t, err)
		assert.Equal(t, "data: OK\n\nevent: error\ndata: {\"error\":\"boom\"}\n\n", string(got))
	})
}

// --- fake OpenAI Responses API -------------------------------------------------

// fakeResponsesAPI is an in-memory OpenAI Responses API.
type fakeResponsesAPI struct {
	mu        sync.Mutex
	calls     []nativeVendorCall
	responses map[string]map[string]any
	server    *httptest.Server
	n         int
	// Files and Batches (native_openai_batches_test.go).
	files       map[string]*fakeOpenAIFile
	batches     map[string]map[string]any
	failContent bool
	// Conversations (native_conversations_test.go).
	conversations map[string]map[string]any
}

func newFakeResponsesAPI(t *testing.T) *fakeResponsesAPI {
	f := &fakeResponsesAPI{responses: map[string]map[string]any{}, files: map[string]*fakeOpenAIFile{}, batches: map[string]map[string]any{},
		conversations: map[string]map[string]any{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeResponsesAPI) recorded() []nativeVendorCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]nativeVendorCall(nil), f.calls...)
}

func (f *fakeResponsesAPI) callsTo(method, path string) []nativeVendorCall {
	var out []nativeVendorCall
	for _, c := range f.recorded() {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// complete ends a background response with usage.
func (f *fakeResponsesAPI) complete(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.responses[id]
	r["status"] = "completed"
	r["completed_at"] = time.Now().Unix()
	r["usage"] = responsesUsage()
}

func responsesUsage() map[string]any {
	return map[string]any{
		"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
		"input_tokens_details": map[string]any{"cached_tokens": 2},
	}
}

func (f *fakeResponsesAPI) notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(openaiVendor.NativeError(http.StatusNotFound, "not_found_error", "Response not found"))
}

func (f *fakeResponsesAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, nativeVendorCall{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Headers: r.Header.Clone(), Body: body})
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		var req struct {
			Model      string `json:"model"`
			Stream     bool   `json:"stream"`
			Background bool   `json:"background"`
			Previous   string `json:"previous_response_id"`
			Conv       any    `json:"conversation"`
		}
		_ = json.Unmarshal(body, &req)
		conv, _ := req.Conv.(string)
		if m, ok := req.Conv.(map[string]any); ok {
			conv, _ = m["id"].(string)
		}
		f.mu.Lock()
		if (req.Previous != "" && f.responses[req.Previous] == nil) || (conv != "" && f.conversations[conv] == nil) {
			f.mu.Unlock()
			f.notFound(w)
			return
		}
		f.n++
		id := fmt.Sprintf("resp_%d", f.n)
		obj := map[string]any{
			"id": id, "object": "response", "created_at": time.Now().Unix(), "model": req.Model + "-2026",
			"status": "completed", "background": req.Background, "service_tier": "default",
			"previous_response_id": nil, "usage": responsesUsage(),
			"output": []any{map[string]any{"type": "message", "id": "msg_" + id, "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "hi"}}}},
		}
		if req.Previous != "" {
			obj["previous_response_id"] = req.Previous
		}
		if conv != "" {
			obj["conversation"] = map[string]any{"id": conv}
		}
		if req.Background {
			obj["status"], obj["usage"] = "queued", nil
		}
		f.responses[id] = obj
		snapshot := func() []byte { b, _ := json.Marshal(obj); return b }
		created := snapshot()
		f.mu.Unlock()
		if !req.Stream {
			_, _ = w.Write(created)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(typ string, data string) {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, data)
			w.(http.Flusher).Flush()
		}
		inProgress := strings.Replace(string(created), `"status":"completed"`, `"status":"in_progress"`, 1)
		if req.Background {
			send("response.queued", `{"type":"response.queued","sequence_number":0,"response":`+string(created)+`}`)
		}
		send("response.created", `{"type":"response.created","sequence_number":1,"response":`+inProgress+`}`)
		send("response.output_text.delta", `{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_`+id+`","delta":"hi"}`)
		if req.Background {
			f.complete(id)
		}
		f.mu.Lock()
		done := snapshot()
		f.mu.Unlock()
		send("response.completed", `{"type":"response.completed","sequence_number":3,"response":`+string(done)+`}`)
	case r.Method == http.MethodPost && (r.URL.Path == "/v1/responses/compact" || r.URL.Path == "/v1/responses/input_tokens"):
		_, _ = w.Write([]byte(`{"object":"response.input_tokens","input_tokens":12}`))
	case strings.HasPrefix(r.URL.Path, "/v1/responses/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/responses/"), "/")
		f.mu.Lock()
		obj, ok := f.responses[id]
		var out []byte
		switch {
		case !ok:
		case r.Method == http.MethodGet && action == "":
			out, _ = json.Marshal(obj)
		case r.Method == http.MethodGet && action == "input_items":
			out = []byte(`{"object":"list","data":[{"id":"msg_in","type":"message"}],"first_id":"msg_in","last_id":"msg_in","has_more":false}`)
		case r.Method == http.MethodPost && action == "cancel":
			obj["status"] = "cancelled"
			out, _ = json.Marshal(obj)
		case r.Method == http.MethodDelete && action == "":
			delete(f.responses, id)
			out = []byte(`{"id":"` + id + `","object":"response","deleted":true}`)
		}
		f.mu.Unlock()
		if out == nil {
			f.notFound(w)
			return
		}
		_, _ = w.Write(out)
	default:
		if !f.serveInputCalls(w, r) && !f.serveConversations(w, r, body) && !f.serveBatches(w, r, body) {
			f.notFound(w)
		}
	}
}

// --- harness -----------------------------------------------------------------

// responsesHarness is the native harness with its OpenAI "primary" LLM backed
// by fakeResponsesAPI, held by App C and by a second App D.
type responsesHarness struct {
	*nativeHarness
	api  *fakeResponsesAPI
	appD *models.App
	keyD string
}

func newResponsesHarness(t *testing.T, opts ...nativeHarnessOpts) *responsesHarness {
	t.Helper()
	var o nativeHarnessOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	h := newNativeHarness(t, o)
	api := newFakeResponsesAPI(t)
	h.primary.APIEndpoint = api.server.URL
	h.primary.Failover = models.LLMFailover{}
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	app, err := h.service.CreateApp("Native D", "", h.appC.UserID, nil, []uint{h.primary.ID}, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, h.service.ActivateAppCredential(app.ID))
	app, err = h.service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NoError(t, h.db.Create(&models.ModelPrice{ModelName: "gpt-4o-2026", Vendor: string(models.OPENAI),
		CPT: 2, CPIT: 1, CacheReadPT: 0.5, Currency: "USD"}).Error)
	return &responsesHarness{nativeHarness: h, api: api, appD: app, keyD: app.Credential.Secret}
}

// wrapResp mints the ID app would have been handed for response vendorID.
func (h *responsesHarness) wrapResp(app *models.App, vendorID string) string {
	h.t.Helper()
	codec, err := nativeapi.CodecForLLM(h.primary, time.Now())
	require.NoError(h.t, err)
	rt, ok := (&openaiVendor.OpenAI{}).NativeAPI().Resource(openaiVendor.ResourceResponse)
	require.True(h.t, ok)
	id, err := codec.Wrap(rt, vendorID, nativeapi.Scope{LLMID: h.primary.ID, AppID: app.ID})
	require.NoError(h.t, err)
	return id
}

func decodeObj(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m), "body: %s", b)
	return m
}

// oaiErr decodes an OpenAI-shaped error body.
func oaiErr(t *testing.T, b []byte) (typ, msg string) {
	t.Helper()
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(b, &e), "body: %s", b)
	return e.Error.Type, e.Error.Message
}

// --- end to end ----------------------------------------------------------------

func TestResponsesChain_CreateWrapsIDAndRecordsUsage(t *testing.T) {
	h := newResponsesHarness(t)
	for _, mode := range []string{"rest", "call"} {
		t.Run(mode, func(t *testing.T) {
			resp, body := h.do(h.keyC, http.MethodPost, "/llm/"+mode+"/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`,
				"Accept-Encoding", "gzip", "OpenAI-Project", "proj_elsewhere", "OpenAI-Organization", "org_elsewhere")
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
			obj := decodeObj(t, body)
			id := obj["id"].(string)
			assert.True(t, strings.HasPrefix(id, "resp_") && strings.Contains(id, ".t1k"), "id not wrapped: %s", id)
			assert.Nil(t, obj["previous_response_id"])
			vendorID, _, _ := strings.Cut(id, ".")
			assert.Equal(t, h.wrapResp(h.appC, vendorID), id)
		})
	}
	creates := h.api.callsTo(http.MethodPost, "/v1/responses")
	require.Len(t, creates, 2)
	for _, c := range creates {
		assert.Empty(t, c.Headers.Get("OpenAI-Project"), "a response is created in the LLM's project")
		assert.Empty(t, c.Headers.Get("OpenAI-Organization"))
	}

	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 2, "each create is one chat record")
	assert.Equal(t, "gpt-4o-2026", recs[0].Name)
	assert.Equal(t, 8, recs[0].PromptTokens, "cached tokens are not counted twice")
	assert.Equal(t, 2, recs[0].CacheReadPromptTokens)
	assert.Equal(t, 5, recs[0].ResponseTokens)
	assert.Greater(t, recs[0].Cost, 0.0)
}

func TestResponsesChain_PreviousResponseOwnership(t *testing.T) {
	h := newResponsesHarness(t)
	_, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
	mine := decodeObj(t, body)["id"].(string)
	vendorID, _, _ := strings.Cut(mine, ".")

	// The App's own response: the vendor sees its raw ID, the client the wrapped one.
	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses",
		`{"model":"gpt-4o","input":"more","previous_response_id":"`+mine+`"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Equal(t, mine, decodeObj(t, body)["previous_response_id"])
	creates := h.api.callsTo(http.MethodPost, "/v1/responses")
	require.Len(t, creates, 2)
	assert.Equal(t, vendorID, decodeObj(t, creates[1].Body)["previous_response_id"])

	refused := []struct{ name, key, ref string }{
		{"another App's response", h.keyD, mine},
		{"a raw vendor ID", h.keyC, vendorID},
		{"a forged ID", h.keyC, vendorID + ".t1kdeadbeef.AAAAAAAAAAAAAAAAAAAAAA"},
		{"a non-string reference", h.keyC, ""},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			ref := `"` + tc.ref + `"`
			if tc.ref == "" {
				ref = `42`
			}
			for _, path := range []string{"/llm/rest/primary/v1/responses", "/llm/call/primary/v1/responses",
				"/llm/rest/primary/v1/responses/compact", "/llm/rest/primary/v1/responses/input_tokens"} {
				before := len(h.api.recorded())
				resp, body := h.do(tc.key, http.MethodPost, path, `{"model":"gpt-4o","input":"x","previous_response_id":`+ref+`}`)
				require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", path, body)
				typ, msg := oaiErr(t, body)
				assert.Equal(t, "invalid_request_error", typ)
				assert.Equal(t, nativeNotFoundMsg, msg)
				assert.Len(t, h.api.recorded(), before, "%s reached the vendor", path)
			}
		})
	}

	// The token count and compaction stay on the synchronous chain.
	resp, body = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses/input_tokens",
		`{"model":"gpt-4o","input":"x","previous_response_id":"`+mine+`"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	counts := h.api.callsTo(http.MethodPost, "/v1/responses/input_tokens")
	require.Len(t, counts, 1)
	assert.Equal(t, vendorID, decodeObj(t, counts[0].Body)["previous_response_id"])
}

func TestResponsesNative_RetrieveCancelInputItemsDelete(t *testing.T) {
	h := newResponsesHarness(t)
	_, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
	first := decodeObj(t, body)["id"].(string)
	_, body = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses",
		`{"model":"gpt-4o","input":"more","previous_response_id":"`+first+`"}`)
	second := decodeObj(t, body)["id"].(string)
	vendorSecond, _, _ := strings.Cut(second, ".")

	resp, body := h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/responses/"+second+"?include[]=message.output_text.logprobs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	obj := decodeObj(t, body)
	assert.Equal(t, second, obj["id"])
	assert.Equal(t, first, obj["previous_response_id"])
	gets := h.api.callsTo(http.MethodGet, "/v1/responses/"+vendorSecond)
	require.Len(t, gets, 1)
	assert.Equal(t, "include%5B%5D=message.output_text.logprobs", gets[0].RawQuery)

	resp, body = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/responses/"+second+"/input_items?limit=5", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Equal(t, "msg_in", decodeObj(t, body)["first_id"])

	// Resuming a stream is not served.
	resp, body = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/responses/"+second+"?stream=true", "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", body)
	_, msg := oaiErr(t, body)
	assert.Contains(t, msg, `"stream"`)

	// Another App, on every operation: the same 404, and the vendor never asked.
	before := len(h.api.recorded())
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/v1/responses/" + second},
		{http.MethodGet, "/v1/responses/" + second + "/input_items"},
		{http.MethodPost, "/v1/responses/" + second + "/cancel"},
		{http.MethodDelete, "/v1/responses/" + second},
		{http.MethodGet, "/v1/responses/" + vendorSecond},
	} {
		resp, body := h.do(h.keyD, c.method, "/llm/call/primary"+c.path, "")
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s %s: %s", c.method, c.path, body)
		typ, _ := oaiErr(t, body)
		assert.Equal(t, "invalid_request_error", typ)
	}
	assert.Len(t, h.api.recorded(), before)

	// The owner deletes a foreground response: no ledger row, so the vendor
	// object is checked first.
	resp, body = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/responses/"+second, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Equal(t, second, decodeObj(t, body)["id"])
	require.Len(t, h.api.callsTo(http.MethodDelete, "/v1/responses/"+vendorSecond), 1)
	resp, _ = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/responses/"+second, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestResponsesChain_StreamWrapsEventIDs(t *testing.T) {
	h := newResponsesHarness(t)
	resp, body := h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/responses", `{"model":"gpt-4o","input":"hi","stream":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	id := h.wrapResp(h.appC, "resp_1")
	stream := string(body)
	assert.NotContains(t, stream, `"id":"resp_1"`, "a raw response ID reached the client")
	assert.Equal(t, 2, strings.Count(stream, `"id":"`+id+`"`), "response.created and response.completed carry the wrapped ID:\n%s", stream)
	assert.Contains(t, stream, `"item_id":"msg_resp_1"`, "other events pass through")
	assert.Contains(t, stream, "event: response.output_text.delta\n")

	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 1, "usage from response.completed")
	assert.Equal(t, 5, recs[0].ResponseTokens)
}

func TestResponsesChain_BackgroundAccountedOnceByReconciler(t *testing.T) {
	h := newResponsesHarness(t)
	store := h.service.NativeResourceStore()
	ctx := context.Background()

	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi","background":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	id := decodeObj(t, body)["id"].(string)
	assert.Equal(t, h.wrapResp(h.appC, "resp_1"), id)

	row, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceResponse, "resp_1")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingPending, row.AccountingState)
	assert.Equal(t, string(nativeapi.StateInProgress), row.State)
	assert.Empty(t, h.chatRecords(h.appC), "a background response is not accounted by the synchronous chain")

	// Not ended: nothing recorded, the next poll is scheduled.
	out, err := h.proxy.accountNativeRow(ctx, row)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountNotEnded, out)

	// Another node or App cannot delete it before it is accounted.
	resp, _ = h.do(h.keyD, http.MethodDelete, "/llm/call/primary/v1/responses/"+id, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	h.api.complete("resp_1")
	h.proxy.reconcileNative(ctx, store, time.Now())
	h.proxy.reconcileNative(ctx, store, time.Now().Add(time.Hour))
	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 1, "accounted exactly once")
	assert.Equal(t, "gpt-4o-2026", recs[0].Name)
	assert.Equal(t, 8, recs[0].PromptTokens)
	assert.Equal(t, 2, recs[0].CacheReadPromptTokens)
	assert.Equal(t, 5, recs[0].ResponseTokens)
	assert.InDelta(t, models.ChatRecordCost(string(models.OPENAI), 2, 1, 0, 0.5, 8, 5, 0, 2), recs[0].Cost, 1e-9,
		"background responses are billed at the list price")
	row, err = store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceResponse, "resp_1")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingAccounted, row.AccountingState)

	// Settled: the owner may delete it.
	resp, body = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/responses/"+id, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	_, err = store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceResponse, "resp_1")
	assert.Error(t, err, "the ledger row goes with the response")
}

func TestResponsesChain_BackgroundDeleteAccountsFirst(t *testing.T) {
	h := newResponsesHarness(t)
	_, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi","background":true}`)
	id := decodeObj(t, body)["id"].(string)

	resp, body := h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/responses/"+id, "")
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", body)
	_, msg := oaiErr(t, body)
	assert.Contains(t, msg, "this response cannot be deleted until it has ended")

	h.api.complete("resp_1")
	resp, body = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/responses/"+id, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	require.Len(t, h.chatRecords(h.appC), 1, "the delete accounted the response first")
}

func TestResponsesChain_BackgroundWithoutLedgerRowIsNotDeletable(t *testing.T) {
	h := newResponsesHarness(t)
	_, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi","background":true}`)
	id := decodeObj(t, body)["id"].(string)
	h.api.complete("resp_1")
	// As on another edge: this node holds no row of it.
	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.primary.ID, h.appC.ID, openaiVendor.ResourceResponse, "resp_1")
	require.NoError(t, err)
	require.NoError(t, h.service.NativeResourceStore().Delete(context.Background(), row.ID))

	resp, body := h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/responses/"+id, "")
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", body)
	_, msg := oaiErr(t, body)
	assert.Contains(t, msg, "only be deleted through the gateway that created it")
	assert.Empty(t, h.api.callsTo(http.MethodDelete, "/v1/responses/resp_1"))
}

func TestResponsesChain_BackgroundStreamRecordedOnce(t *testing.T) {
	h := newResponsesHarness(t)
	resp, body := h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/responses",
		`{"model":"gpt-4o","input":"hi","stream":true,"background":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Equal(t, 3, strings.Count(string(body), `"id":"`+h.wrapResp(h.appC, "resp_1")+`"`), "%s", body)

	rows, err := h.service.NativeResourceStore().List(context.Background(), services.NativeResourceListQuery{
		LLMID: h.primary.ID, AppID: h.appC.ID, ResourceType: openaiVendor.ResourceResponse, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1, "queued, created and completed events record the response once")
	assert.Empty(t, h.chatRecords(h.appC), "the reconciler accounts it")

	out, err := h.proxy.accountNativeRow(context.Background(), &rows[0])
	require.NoError(t, err)
	assert.Equal(t, nativeAccountAccounted, out)
	require.Len(t, h.chatRecords(h.appC), 1)
}

func TestResponsesChain_RefusesBackgroundWhenLedgerUnavailable(t *testing.T) {
	h := newResponsesHarness(t)
	// Make every ledger write fail.
	require.NoError(t, h.db.Exec("DROP TABLE native_resources").Error)

	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi","background":true}`)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "%s", body)
	_, msg := oaiErr(t, body)
	assert.Contains(t, msg, "the response could not be recorded by the gateway and was canceled")
	assert.NotContains(t, string(body), "resp_1")
	require.Len(t, h.api.callsTo(http.MethodPost, "/v1/responses/resp_1/cancel"), 1)

	// A foreground response needs no ledger.
	resp, body = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)

	// A stream ends with an error event instead of the raw ID.
	resp, body = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/responses",
		`{"model":"gpt-4o","input":"hi","stream":true,"background":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "event: error\n")
	assert.Contains(t, string(body), "could not be recorded")
	assert.NotContains(t, string(body), `"id":"resp_3"`)
}

func TestResponsesNative_ChainHookPathsStayOnTheChain(t *testing.T) {
	h := newResponsesHarness(t)
	// /v1/responses/compact matches the shape of /v1/responses/{id}, but
	// the chain hook keeps it on the synchronous chain rather than a 405.
	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses/compact", `{"model":"gpt-4o","input":"x"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	// Other methods on a native path are the vendor-shaped 405.
	resp, body = h.do(h.keyC, http.MethodPut, "/llm/call/primary/v1/responses/"+h.wrapResp(h.appC, "resp_9"), "{}")
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "%s", body)
	assert.Equal(t, "GET, DELETE", resp.Header.Get("Allow"))
}

func TestAnthropicMessagesChain_RefusesDuplicateKeys(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	refused := map[string]string{
		"top-level duplicate model": `{"model":"claude-luna","model":"claude-other","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`,
		"case-variant messages":     `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"ok"}],"Messages":[{"role":"user","content":"evil"}]}`,
		"nested duplicate content":  `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"ok","content":"evil"}]}`,
		"case-variant content":      `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"ok","Content":"evil"}]}`,
		"nested exact duplicate text": `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user",` +
			`"content":[{"type":"text","text":"ok","text":"evil"}]}]}`,
		"duplicate in a streamed call": `{"model":"claude-luna","max_tokens":5,"stream":true,"messages":[{"role":"user","content":"a","content":"b"}]}`,
	}
	for name, body := range refused {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{"rest", "call"} {
				before := len(h.vendor.callsTo(http.MethodPost, "/v1/messages"))
				resp, out := h.do(h.keyA, http.MethodPost, "/llm/"+mode+"/claude/v1/messages", body)
				require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", mode, out)
				typ, msg := nativeErr(t, out)
				assert.Equal(t, "invalid_request_error", typ)
				assert.Contains(t, msg, "invalid request body: duplicate key")
				assert.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages"), before, "%s reached the vendor", mode)
			}
		})
	}

	// The same key in sibling objects is a normal conversation, and case
	// variants in free-form subtrees (a tool's JSON Schema properties, a
	// tool call's input, a content block the gateway reads as a map) are
	// different keys to the gateway and the vendor alike.
	for name, body := range map[string]string{
		"siblings": `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]}`,
		"schema properties id and ID": `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"hi"}],` +
			`"tools":[{"name":"lookup","description":"x","input_schema":{"type":"object","properties":{"id":{"type":"string"},"ID":{"type":"string"}}}}]}`,
		"tool_use input Name and name": `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"lookup","input":{"Name":"a","name":"b"}}]},` +
			`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`,
		"content block text and Text": `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":[{"type":"text","text":"ok","Text":"x"}]}]}`,
	} {
		for _, mode := range []string{"rest", "call"} {
			resp, out := h.do(h.keyA, http.MethodPost, "/llm/"+mode+"/claude/v1/messages", body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s %s: %s", name, mode, out)
		}
	}
	var resp *http.Response
	var out []byte
	resp, out = h.do(h.keyA, http.MethodPost, "/llm/call/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":5,"stream":true,"messages":[{"role":"user","content":"a"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.Contains(t, string(out), "message_stop")
}

func TestNativeBatchCreate_NestedDuplicateKeyRefusedOnce(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	resp, out := h.do(h.keyA, http.MethodPost, "/llm/call/claude/v1/messages/batches",
		`{"requests":[{"custom_id":"a","params":{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"ok","Content":"evil"}]}}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	_, msg := nativeErr(t, out)
	assert.Equal(t, `invalid message batch body: request 0: params: duplicate key "Content"`, msg)
	assert.Empty(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/batches"))
}

// Another spelling of a declared path (case, empty segments) is refused, not
// forwarded around the native handling to an upstream that might route it
// case-insensitively.
func TestNativePathVariantsRefused(t *testing.T) {
	h := newResponsesHarness(t)
	_, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
	vendorID := vendorPart(decodeObj(t, body)["id"].(string))

	before := len(h.api.recorded())
	for _, path := range []string{
		"/llm/rest/primary/v1/Responses", "/llm/rest/primary/V1/responses", "/llm/call/primary/v1/responses/",
		"/llm/rest/primary/v1/Responses/input_tokens", "/llm/call/primary/v1/Responses/" + vendorID,
	} {
		resp, out := h.do(h.keyD, http.MethodPost, path, `{"model":"gpt-4o","input":"x","previous_response_id":"`+vendorID+`"}`)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", path, out)
		typ, _ := oaiErr(t, out)
		assert.Equal(t, "invalid_request_error", typ, path)
	}
	assert.Len(t, h.api.recorded(), before, "no variant reached the vendor")

	// The exact path with another method is still the legacy chain's answer.
	resp, _ := h.do(h.keyC, http.MethodGet, "/llm/rest/primary/v1/responses", "")
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	// A case variant of a reference field is refused, not forwarded unverified.
	resp, out := h.do(h.keyD, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"x","Previous_Response_Id":"`+vendorID+`"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	_, msg := oaiErr(t, out)
	assert.Contains(t, msg, `unsupported parameter "Previous_Response_Id"`)
	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches", `{"Input_File_Id":"file-x","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Len(t, h.api.recorded(), before)

	// Anthropic: the batches and messages paths alike.
	for _, path := range []string{"/llm/call/claude/v1/Messages/Batches", "/llm/rest/claude/v1/Messages", "/llm/rest/claude/v1/messages/"} {
		resp, out := h.do(h.keyA, http.MethodPost, path, `{"model":"claude-luna","max_tokens":5,"messages":[{"role":"user","content":"hi"}],"requests":[]}`)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", path, out)
		typ, _ := nativeErr(t, out)
		assert.Equal(t, "not_found_error", typ)
	}
	assert.Empty(t, h.vendor.recorded())
}

// On an endpoint with a version path (.../v1), an upper-case version
// segment does not overlap it and would be forwarded as /v1/V1/responses: the
// caller's own path is fold-checked too.
func TestNativePathVariantsRefused_VersionedEndpoint(t *testing.T) {
	h := newResponsesHarness(t)
	h.primary.APIEndpoint = h.api.server.URL + "/v1"
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	vendorID := vendorPart(decodeObj(t, body)["id"].(string))
	before := len(h.api.recorded())
	for _, path := range []string{"/llm/rest/primary/V1/responses", "/llm/call/primary/V1/Responses/input_tokens", "/llm/call/primary/V1/responses/" + vendorID} {
		resp, out := h.do(h.keyD, http.MethodPost, path, `{"model":"gpt-4o","input":"x","previous_response_id":"`+vendorID+`"}`)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", path, out)
	}
	assert.Len(t, h.api.recorded(), before, "nothing reached the vendor")
}
