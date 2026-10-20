package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A response hook that only labels a buffered response leaves the vendor's
// repeated headers alone: both Set-Cookie lines and the Vary list reach the
// client. The hook returns the full set with one value per key, and
// replacing the set with it kept only the first of each.
func TestBufferedResponse_LabelHookKeepsRepeatedHeaders(t *testing.T) {
	manager := NewDefaultResponseHookManager()
	manager.AddHook(&headerHook{set: map[string]string{"X-Tyk-Compression": "applied"}})
	p := &Proxy{responseHookManager: manager}

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/llm/v1/chat/completions", nil)
	r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
	client := httptest.NewRecorder()
	c := newBufferedResponseCapture(client)
	c.Header().Set("Content-Type", "application/json")
	c.Header().Add("Set-Cookie", "__cf_bm=abc; Path=/")
	c.Header().Add("Set-Cookie", "affinity=node-2; Path=/")
	c.Header().Add("Vary", "Origin")
	c.Header().Add("Vary", "Accept-Encoding")
	c.WriteHeader(http.StatusOK)
	_, _ = c.Write([]byte(`{"answer":true}`))

	llm := &models.LLM{ID: 3, Name: "llm", Vendor: models.OPENAI}
	require.NoError(t, p.executeBufferedResponseHooks(c, llm, &models.App{ID: 1}, r))
	c.WriteToClient()

	resp := client.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))
	assert.Equal(t, []string{"__cf_bm=abc; Path=/", "affinity=node-2; Path=/"}, resp.Header.Values("Set-Cookie"))
	assert.Equal(t, []string{"Origin", "Accept-Encoding"}, resp.Header.Values("Vary"))
	assert.Equal(t, `{"answer":true}`, client.Body.String())
}

func capturedWith(h http.Header) *bufferedResponseCapture {
	c := newBufferedResponseCapture(httptest.NewRecorder())
	for k, vs := range h {
		c.header[k] = append([]string(nil), vs...)
	}
	return c
}

// ModifyHeaders applies what a hook changed: an unchanged key keeps all its
// values, a changed or new key is set, a key the hook left out is removed.
func TestBufferedCapture_ModifyHeadersAppliesOnlyChanges(t *testing.T) {
	c := capturedWith(http.Header{
		"Set-Cookie":   {"a=1", "b=2"},
		"Vary":         {"Origin", "Accept-Encoding"},
		"Content-Type": {"application/json"},
		"X-Drop":       {"gone"},
	})
	c.ModifyHeaders(map[string]string{
		"Set-Cookie":   "a=1",
		"vary":         "Origin", // another case, same value: unchanged
		"Content-Type": "application/json; charset=utf-8",
		"X-Cache":      "MISS",
	})
	assert.Equal(t, http.Header{
		"Set-Cookie":   {"a=1", "b=2"},
		"Vary":         {"Origin", "Accept-Encoding"},
		"Content-Type": {"application/json; charset=utf-8"},
		"X-Cache":      {"MISS"},
	}, c.header)
}

// A hook that answers with a header set of its own, as a cache serving a
// stored response does, still gets exactly that set.
func TestBufferedCapture_ModifyHeadersFullReplacementStillWorks(t *testing.T) {
	c := capturedWith(http.Header{
		"Set-Cookie":    {"a=1", "b=2"},
		"Content-Type":  {"text/event-stream"},
		"X-Request-Id":  {"upstream-1"},
		"Cache-Control": {"no-cache"},
	})
	c.ModifyHeaders(map[string]string{
		"Content-Type": "application/json",
		"X-Cache":      "HIT",
		"X-Request-Id": "stored-7",
	})
	assert.Equal(t, http.Header{
		"Content-Type": {"application/json"},
		"X-Cache":      {"HIT"},
		"X-Request-Id": {"stored-7"},
	}, c.header)
}

// decodedBodyHook rewrites the body in the write phase and returns the headers it
// was given, which omit Content-Encoding (the body it sees is decoded).
type decodedBodyHook struct{ headerHook }

func (*decodedBodyHook) OnBeforeWrite(_ context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	return &ResponseWriteResponse{Modified: true, Body: []byte(`{"rewritten":true}`), Headers: req.Headers}, nil
}

// A write hook that replaces the (decoded) body drops Content-Encoding, as it
// did when the header set was replaced, and leaves repeated headers alone.
func TestBufferedResponse_BodyHookDropsContentEncoding(t *testing.T) {
	manager := NewDefaultResponseHookManager()
	manager.AddHook(&decodedBodyHook{})
	p := &Proxy{responseHookManager: manager}

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/llm/v1/chat/completions", nil)
	r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
	c := newBufferedResponseCapture(httptest.NewRecorder())
	c.Header().Set("Content-Type", "application/json")
	c.Header().Set("Content-Encoding", "gzip")
	c.Header().Add("Set-Cookie", "a=1")
	c.Header().Add("Set-Cookie", "b=2")
	c.WriteHeader(http.StatusOK)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(`{"upstream":true}`))
	require.NoError(t, zw.Close())
	_, _ = c.Write(gz.Bytes())

	llm := &models.LLM{ID: 3, Name: "llm", Vendor: models.OPENAI}
	require.NoError(t, p.executeBufferedResponseHooks(c, llm, &models.App{ID: 1}, r))
	assert.Empty(t, c.header.Get("Content-Encoding"))
	assert.Equal(t, []string{"a=1", "b=2"}, c.header.Values("Set-Cookie"))
	assert.Equal(t, `{"rewritten":true}`, c.buffer.String())
}
