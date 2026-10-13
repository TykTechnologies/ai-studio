package openaiVendor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

func TestNativeSpecValid(t *testing.T) {
	m, err := nativeapi.NewMatcher((&OpenAI{}).NativeAPI())
	require.NoError(t, err)

	cases := []struct{ method, path, op string }{
		{http.MethodGet, "/v1/responses/resp_1", "openai.responses.retrieve"},
		{http.MethodDelete, "/v1/responses/resp_1", "openai.responses.delete"},
		{http.MethodPost, "/v1/responses/resp_1/cancel", "openai.responses.cancel"},
		{http.MethodGet, "/v1/responses/resp_1/input_items", "openai.responses.input_items"},
		// The creates stay on the synchronous chain.
		{http.MethodPost, "/v1/responses", ""},
		{http.MethodPost, "/v1/chat/completions", ""},
	}
	for _, c := range cases {
		got, ok := m.Match(c.method, c.path)
		if c.op == "" {
			assert.False(t, ok, "%s %s", c.method, c.path)
			continue
		}
		require.True(t, ok, "%s %s", c.method, c.path)
		assert.Equal(t, c.op, got.Op.Name)
	}

	hooks := m.Spec().ChainHooks
	for _, p := range []string{"/v1/responses", "/v1/responses/input_tokens", "/v1/responses/compact"} {
		h, ok := hooks.Hook(http.MethodPost, p)
		require.True(t, ok, p)
		assert.Equal(t, "previous_response_id", h.RequestRefs[0].Path)
	}
	create, _ := hooks.Hook(http.MethodPost, "/v1/responses")
	assert.Equal(t, "response", create.EventObject)
	assert.Equal(t, ResourceResponse, create.ResponseIDs[0].Resource)

	rt, ok := m.Spec().Resource(ResourceResponse)
	require.True(t, ok)
	assert.True(t, rt.ValidVendorID("resp_67ccd2bed1ec8190b14f964abc0542670bb6a6b452d3795b"))
	assert.False(t, rt.ValidVendorID("msg_1"))
	assert.False(t, rt.ValidVendorID("resp_../x"))
}

func responseJSON(t *testing.T, mut func(map[string]any)) []byte {
	t.Helper()
	obj := map[string]any{
		"id": "resp_1", "object": "response", "status": "completed", "background": false,
		"model": "gpt-4o-2024-08-06", "created_at": 1760000000, "completed_at": 1760000060,
		"service_tier": "default",
		"output": []any{
			map[string]any{"type": "function_call", "id": "fc_1"},
			map[string]any{"type": "message", "id": "msg_1"},
		},
		"usage": map[string]any{
			"input_tokens": 100, "output_tokens": 20,
			"input_tokens_details":  map[string]any{"cached_tokens": 40},
			"output_tokens_details": map[string]any{"reasoning_tokens": 5},
		},
	}
	if mut != nil {
		mut(obj)
	}
	b, err := json.Marshal(obj)
	require.NoError(t, err)
	return b
}

func TestResponseAsync(t *testing.T) {
	rt, _ := nativeSpec.Resource(ResourceResponse)
	a := rt.Async

	assert.False(t, a.IsTracked(responseJSON(t, nil)), "a foreground response is accounted synchronously")
	assert.True(t, a.IsTracked(responseJSON(t, func(o map[string]any) { o["background"] = true })))
	assert.False(t, a.IsTracked([]byte(`{"background":true}`)), "not a response object")

	for status, want := range map[string]nativeapi.ResourceState{
		"queued": nativeapi.StateInProgress, "in_progress": nativeapi.StateInProgress,
		"completed": nativeapi.StateEnded, "failed": nativeapi.StateEnded,
		"cancelled": nativeapi.StateEnded, "incomplete": nativeapi.StateEnded,
	} {
		info, err := a.State(responseJSON(t, func(o map[string]any) { o["status"] = status }))
		require.NoError(t, err)
		assert.Equal(t, want, info.State, status)
		require.NotNil(t, info.ResultsExpireAt)
		assert.Equal(t, time.Unix(1760000000, 0).UTC().Add(30*24*time.Hour), *info.ResultsExpireAt)
	}
	info, _ := a.State(responseJSON(t, nil))
	require.NotNil(t, info.EndedAt)
	assert.Equal(t, time.Unix(1760000060, 0).UTC(), *info.EndedAt)
	_, err := a.State([]byte(`{"object":"list"}`))
	assert.Error(t, err)

	u, ok, err := a.ObjectUsage(responseJSON(t, func(o map[string]any) { o["background"] = true }))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, nativeapi.Usage{Model: "gpt-4o-2024-08-06", InputTokens: 100, OutputTokens: 20,
		CacheReadInputTokens: 40, Tier: "", Billed: true}, u, `"default" is list price: recorded as no tier`)
	assert.Equal(t, 1.0, u.PriceMultiplier(), "background responses are billed at the list price")

	_, ok, err = a.ObjectUsage(responseJSON(t, func(o map[string]any) { o["status"] = "cancelled"; o["usage"] = nil }))
	require.NoError(t, err)
	assert.False(t, ok, "a response cancelled before it ran has no usage")

	assert.Equal(t, nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/responses/resp_1"}, a.Poll("resp_1"))
	assert.Equal(t, nativeapi.UpstreamReq{Method: http.MethodPost, Path: "/v1/responses/resp_1/cancel"}, a.Cancel("resp_1"))
}

func TestNativeErrorShape(t *testing.T) {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Param   any    `json:"param"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(NativeError(404, "not_found_error", "resource not found"), &e))
	assert.Equal(t, "resource not found", e.Error.Message)
	assert.Equal(t, "invalid_request_error", e.Error.Type)
	require.NoError(t, json.Unmarshal(NativeError(502, "api_error", "x"), &e))
	assert.Equal(t, "server_error", e.Error.Type)
}

func TestAnalyzeResponsesAPI(t *testing.T) {
	v := &OpenAI{}
	llm, app := &models.LLM{ID: 1, Vendor: models.OPENAI}, &models.App{ID: 2}
	req := func(path string) *http.Request { return httptest.NewRequest(http.MethodPost, path, nil) }

	_, _, resp, err := v.AnalyzeResponse(llm, app, 200, responseJSON(t, nil), req("/llm/rest/gpt/v1/responses"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "gpt-4o-2024-08-06", resp.GetModel())
	assert.Equal(t, 100, resp.GetPromptTokens(), "input_tokens includes the cached tokens, as prompt_tokens does")
	assert.Equal(t, 40, resp.GetCacheReadPromptTokens())
	assert.Equal(t, 20, resp.GetResponseTokens())
	assert.Equal(t, 1, resp.GetToolCount())

	// A background response is accounted by the native reconciler.
	_, _, resp, err = v.AnalyzeResponse(llm, app, 200,
		responseJSON(t, func(o map[string]any) { o["background"] = true; o["status"] = "queued"; o["usage"] = nil }),
		req("/llm/rest/gpt/v1/responses"))
	require.NoError(t, err)
	assert.Nil(t, resp)

	// Errors and token counts carry no usage; compaction does.
	_, _, resp, err = v.AnalyzeResponse(llm, app, 404, []byte(`{"error":{"message":"x"}}`), req("/llm/rest/gpt/v1/responses"))
	require.NoError(t, err)
	assert.Nil(t, resp)
	_, _, resp, err = v.AnalyzeResponse(llm, app, 200, []byte(`{"object":"response.input_tokens","input_tokens":12}`),
		req("/llm/rest/gpt/v1/responses/input_tokens"))
	require.NoError(t, err)
	assert.Nil(t, resp)
	_, _, resp, err = v.AnalyzeResponse(llm, app, 200, responseJSON(t, func(o map[string]any) { o["object"] = "response.compaction" }),
		req("/llm/rest/gpt/v1/responses/compact"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 20, resp.GetResponseTokens())

	// Chat completions are unchanged.
	_, _, resp, err = v.AnalyzeResponse(llm, app, 200, []byte(`{"model":"gpt-4o","usage":{"prompt_tokens":3,"completion_tokens":4}}`),
		req("/llm/rest/gpt/v1/chat/completions"))
	require.NoError(t, err)
	assert.Equal(t, 3, resp.GetPromptTokens())
}

func TestAnalyzeResponsesAPIStream(t *testing.T) {
	v := &OpenAI{}
	llm, app := &models.LLM{ID: 1, Vendor: models.OPENAI}, &models.App{ID: 2}
	r := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", nil)
	completed := responseJSON(t, nil)
	stream := "event: response.created\n" +
		`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","status":"in_progress","usage":null}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hi"}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":2,"response":` + string(completed) + "}\n\n"

	_, _, resp, err := v.AnalyzeStreamingResponse(llm, app, 200, []byte(stream), r, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 100, resp.GetPromptTokens())
	assert.Equal(t, 20, resp.GetResponseTokens())

	bg := "event: response.queued\n" +
		`data: {"type":"response.queued","response":{"id":"resp_2","object":"response","status":"queued","background":true,"usage":null}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":` + string(responseJSON(t, func(o map[string]any) { o["background"] = true })) + "}\n\n"
	_, _, resp, err = v.AnalyzeStreamingResponse(llm, app, 200, []byte(bg), r, nil)
	require.NoError(t, err)
	assert.Nil(t, resp, "a streamed background response is accounted by the native reconciler")
}

func TestConversationSpec(t *testing.T) {
	m, err := nativeapi.NewMatcher((&OpenAI{}).NativeAPI())
	require.NoError(t, err)
	for _, c := range []struct{ method, path, op string }{
		{http.MethodPost, "/v1/conversations", "openai.conversations.create"},
		{http.MethodGet, "/v1/conversations/conv_1", "openai.conversations.retrieve"},
		{http.MethodPost, "/v1/conversations/conv_1", "openai.conversations.update"},
		{http.MethodDelete, "/v1/conversations/conv_1", "openai.conversations.delete"},
		{http.MethodPost, "/v1/conversations/conv_1/items", "openai.conversations.items.create"},
		{http.MethodGet, "/v1/conversations/conv_1/items", "openai.conversations.items.list"},
		{http.MethodGet, "/v1/conversations/conv_1/items/msg_1", "openai.conversations.items.retrieve"},
		{http.MethodDelete, "/v1/conversations/conv_1/items/msg_1", "openai.conversations.items.delete"},
	} {
		got, ok := m.Match(c.method, c.path)
		require.True(t, ok, "%s %s", c.method, c.path)
		assert.Equal(t, c.op, got.Op.Name)
	}
	create, _ := m.Spec().ChainHooks.Hook(http.MethodPost, "/v1/responses")
	assert.Contains(t, create.RequestRefs, conversationRef)
	assert.Contains(t, create.ResponseIDs, conversationRef)
	assert.True(t, conversationRef.ObjectID)
}

func TestSplitJoinConversationItems(t *testing.T) {
	body := `{"metadata":{"n":1},"items":[{"role":"user","content":"my SECRET"}]}`
	items, err := splitConversationItems([]byte(body))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, ResponsesFilterFormat, items[0].Format)
	assert.Empty(t, items[0].Model, "storing items invokes no model")
	assert.JSONEq(t, `{"input":[{"role":"user","content":"my SECRET"}]}`, string(items[0].Body))

	items[0].Body = []byte(`{"input":[{"role":"user","content":"my [REDACTED]"}]}`)
	out, err := joinConversationItems([]byte(body), items)
	require.NoError(t, err)
	assert.JSONEq(t, `{"metadata":{"n":1},"items":[{"role":"user","content":"my [REDACTED]"}]}`, string(out))

	items, err = splitConversationItems([]byte(`{"metadata":{}}`))
	require.NoError(t, err)
	assert.Empty(t, items, "no items, nothing to screen")

	for _, b := range []string{`nope`, `{"items":{}}`, `{"items":"x"}`, `{"items":[],"Items":[]}`,
		`{"items":[{"role":"user","content":"a","content":"b"}]}`} {
		_, err := splitConversationItems([]byte(b))
		assert.ErrorIs(t, err, errConversationBody, b)
	}
}
