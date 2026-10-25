package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveConversations answers the Conversations API; false when the path is
// not one of its.
func (f *fakeResponsesAPI) serveConversations(w http.ResponseWriter, r *http.Request, body []byte) bool {
	if !strings.HasPrefix(r.URL.Path, "/v1/conversations") {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	writeJSON := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(b)
	}
	if r.Method == http.MethodPost && r.URL.Path == "/v1/conversations" {
		var req struct {
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(body, &req)
		f.n++
		id := fmt.Sprintf("conv_%d", f.n)
		f.conversations[id] = map[string]any{"id": id, "object": "conversation", "created_at": time.Now().Unix(), "metadata": map[string]any{}, "items": req.Items}
		writeJSON(map[string]any{"id": id, "object": "conversation", "created_at": time.Now().Unix(), "metadata": map[string]any{}})
		return true
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/conversations/")
	id, sub, _ := strings.Cut(rest, "/")
	c, ok := f.conversations[id]
	if !ok {
		f.notFound(w)
		return true
	}
	view := map[string]any{"id": id, "object": "conversation", "created_at": c["created_at"], "metadata": c["metadata"]}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		writeJSON(view)
	case sub == "" && r.Method == http.MethodPost:
		writeJSON(view)
	case sub == "" && r.Method == http.MethodDelete:
		delete(f.conversations, id)
		writeJSON(map[string]any{"id": id, "object": "conversation.deleted", "deleted": true})
	case sub == "items" && r.Method == http.MethodGet:
		writeJSON(map[string]any{"object": "list", "data": c["items"], "first_id": "msg_1", "last_id": "msg_1", "has_more": false})
	case sub == "items" && r.Method == http.MethodPost:
		var req struct {
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(body, &req)
		items, _ := c["items"].([]any)
		c["items"] = append(items, req.Items...)
		writeJSON(map[string]any{"object": "list", "data": req.Items, "has_more": false})
	case strings.HasPrefix(sub, "items/") && r.Method == http.MethodGet:
		writeJSON(map[string]any{"type": "message", "id": strings.TrimPrefix(sub, "items/"), "role": "user", "content": []any{}})
	case strings.HasPrefix(sub, "items/") && r.Method == http.MethodDelete:
		writeJSON(view)
	default:
		f.notFound(w)
	}
	return true
}

func TestConversations_Ownership(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)

	resp, out := h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/conversations",
		`{"items":[{"type":"message","role":"user","content":"hello"}],"metadata":{"topic":"x"}}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	conv := decodeObj(t, out)["id"].(string)
	assert.Contains(t, conv, ".t1k", "the conversation ID is signed")
	vendorConv := vendorPart(conv)
	assert.Equal(t, "conv_1", vendorConv)

	// The owner uses it everywhere: string and object forms on responses,
	// retrieve, update, items, delete.
	for _, ref := range []string{`"` + conv + `"`, `{"id":"` + conv + `"}`} {
		resp, out = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"more","conversation":`+ref+`}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
		got := decodeObj(t, out)["conversation"].(map[string]any)["id"]
		assert.Equal(t, conv, got, "the response's conversation is wrapped")
	}
	creates := h.api.callsTo(http.MethodPost, "/v1/responses")
	require.Len(t, creates, 2)
	assert.Equal(t, vendorConv, decodeObj(t, creates[0].Body)["conversation"])
	assert.Equal(t, map[string]any{"id": vendorConv}, decodeObj(t, creates[1].Body)["conversation"])

	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/conversations/" + conv, ""},
		{http.MethodPost, "/v1/conversations/" + conv, `{"metadata":{"topic":"y"}}`},
		{http.MethodGet, "/v1/conversations/" + conv + "/items?limit=10", ""},
		{http.MethodPost, "/v1/conversations/" + conv + "/items", `{"items":[{"role":"user","content":"again"}]}`},
		{http.MethodGet, "/v1/conversations/" + conv + "/items/msg_1", ""},
		{http.MethodDelete, "/v1/conversations/" + conv + "/items/msg_1", ""},
	} {
		resp, out = h.do(h.keyC, c.method, "/llm/call/primary"+c.path, c.body)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s %s: %s", c.method, c.path, out)
		if id, ok := decodeObj(t, out)["id"].(string); ok && strings.HasPrefix(id, "conv_") {
			assert.Equal(t, conv, id, "%s %s", c.method, c.path)
		}
	}

	// Another App, a raw ID or a forged one: the same 404, never forwarded.
	before := len(h.api.recorded())
	for _, ref := range []string{conv, vendorConv} {
		key := h.keyD
		if ref == vendorConv {
			key = h.keyC
		}
		for _, c := range []struct{ method, path, body string }{
			{http.MethodPost, "/v1/responses", `{"model":"gpt-4o","input":"x","conversation":"` + ref + `"}`},
			{http.MethodPost, "/v1/responses", `{"model":"gpt-4o","input":"x","conversation":{"id":"` + ref + `"}}`},
			{http.MethodPost, "/v1/responses/input_tokens", `{"model":"gpt-4o","input":"x","conversation":"` + ref + `"}`},
			{http.MethodGet, "/v1/conversations/" + ref, ""},
			{http.MethodPost, "/v1/conversations/" + ref, `{"metadata":{}}`},
			{http.MethodDelete, "/v1/conversations/" + ref, ""},
			{http.MethodGet, "/v1/conversations/" + ref + "/items", ""},
			{http.MethodPost, "/v1/conversations/" + ref + "/items", `{"items":[]}`},
			{http.MethodGet, "/v1/conversations/" + ref + "/items/msg_1", ""},
			{http.MethodDelete, "/v1/conversations/" + ref + "/items/msg_1", ""},
		} {
			resp, out = h.do(key, c.method, "/llm/call/primary"+c.path, c.body)
			require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s %s: %s", c.method, c.path, out)
		}
	}
	resp, _ = h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"x","conversation":{"id":5}}`)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "a malformed reference is not forwarded")
	assert.Len(t, h.api.recorded(), before)

	resp, out = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/conversations/"+conv, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.Equal(t, conv, decodeObj(t, out)["id"])
}
