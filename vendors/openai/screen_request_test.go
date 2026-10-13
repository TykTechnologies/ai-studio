package openaiVendor

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The pass-through screen only decides about streaming, so it must accept
// every message shape the API does: text and image parts, array tool
// content, assistant messages with null content. It used to decode content
// as a string and answer 400 for any multi-part message.
func TestProxyScreenRequest_AcceptsArrayContent(t *testing.T) {
	body := `{"model":"gpt-4o","messages":[` +
		`{"role":"user","content":[{"type":"text","text":"what is this?"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":[{"type":"text","text":"result"}]}]}`
	r := httptest.NewRequest("POST", "/llm/rest/x/v1/chat/completions", strings.NewReader(body))
	require.NoError(t, New().ProxyScreenRequest(&models.LLM{}, r, false))

	// The screen leaves the body for the handler to read.
	got, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestProxyScreenRequest_StreamingRules(t *testing.T) {
	screen := func(body string, streamingChannel bool) error {
		r := httptest.NewRequest("POST", "/llm/rest/x/v1/chat/completions", strings.NewReader(body))
		return New().ProxyScreenRequest(&models.LLM{}, r, streamingChannel)
	}
	assert.NoError(t, screen(`{"stream":true,"messages":[]}`, true))
	assert.Error(t, screen(`{"stream":false,"messages":[]}`, true), "the streaming endpoint requires stream")
	assert.Error(t, screen(`{"stream":true,"messages":[]}`, false), "the REST endpoint refuses stream")
	assert.Error(t, screen(`not json`, false))
}
