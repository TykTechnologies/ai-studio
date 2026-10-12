package anthropicVendor

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

func TestNativeAPISpecValid(t *testing.T) {
	m, err := nativeapi.NewMatcher((&Anthropic{}).NativeAPI())
	require.NoError(t, err)
	for _, tc := range []struct{ method, path, op string }{
		{http.MethodPost, "/v1/messages/batches", "anthropic.message_batches.create"},
		{http.MethodGet, "/v1/messages/batches/x", "anthropic.message_batches.retrieve"},
		{http.MethodPost, "/v1/messages/batches/x/cancel", "anthropic.message_batches.cancel"},
	} {
		got, ok := m.Match(tc.method, tc.path)
		require.True(t, ok, tc.path)
		assert.Equal(t, tc.op, got.Op.Name)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1/messages/count_tokens"},
		{http.MethodGet, "/v1/models"},
	} {
		_, ok := m.Match(tc.method, tc.path)
		assert.False(t, ok, tc.path)
	}
}

const batchBody = `{
  "requests": [
    {"custom_id": "a", "params": {"model": "claude-luna", "max_tokens": 10,
       "messages": [{"role": "user", "content": "SECRET one"}]}},
    {"params": {"model":"claude-sol","max_tokens":5,"messages":[{"role":"user","content":"two"}]}, "custom_id": "b"}
  ],
  "extra": 1
}`

func TestSplitJoinBatch(t *testing.T) {
	items, err := splitBatch([]byte(batchBody))
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "a", items[0].Key)
	assert.Equal(t, "claude-luna", items[0].Model)
	assert.Equal(t, "anthropic", items[0].Format)
	assert.True(t, strings.HasPrefix(string(items[0].Body), `{"model": "claude-luna"`), string(items[0].Body))
	assert.True(t, strings.HasSuffix(string(items[0].Body), `}]}`), string(items[0].Body))
	assert.Equal(t, "b", items[1].Key)
	assert.Equal(t, `{"model":"claude-sol","max_tokens":5,"messages":[{"role":"user","content":"two"}]}`, string(items[1].Body))

	// Unchanged items join back to the original, byte for byte.
	same, err := joinBatch([]byte(batchBody), items)
	require.NoError(t, err)
	assert.Equal(t, batchBody, string(same))

	// A changed item is spliced in; everything else is untouched.
	changed := append([]nativeapi.Item(nil), items...)
	changed[0].Body = bytes.ReplaceAll(items[0].Body, []byte("SECRET"), []byte("[REDACTED]"))
	out, err := joinBatch([]byte(batchBody), changed)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(batchBody, "SECRET", "[REDACTED]", 1), string(out))
}

func TestSplitBatchRejects(t *testing.T) {
	for name, body := range map[string]string{
		"not json":               `nope`,
		"no requests":            `{"other":[]}`,
		"empty requests":         `{"requests":[]}`,
		"requests not array":     `{"requests":{}}`,
		"missing custom_id":      `{"requests":[{"params":{"model":"m"}}]}`,
		"empty custom_id":        `{"requests":[{"custom_id":"","params":{"model":"m"}}]}`,
		"missing params":         `{"requests":[{"custom_id":"a"}]}`,
		"params not object":      `{"requests":[{"custom_id":"a","params":"x"}]}`,
		"missing model":          `{"requests":[{"custom_id":"a","params":{"max_tokens":1}}]}`,
		"duplicate model":        `{"requests":[{"custom_id":"a","params":{"model":"ok","model":"evil"}}]}`,
		"case-variant messages":  `{"requests":[{"custom_id":"a","params":{"model":"m","messages":[],"Messages":[]}}]}`,
		"case-variant system":    `{"requests":[{"custom_id":"a","params":{"model":"m","system":"x","SYSTEM":"y"}}]}`,
		"long-s folded messages": `{"requests":[{"custom_id":"a","params":{"model":"m","messages":[],"meſſages":[]}}]}`,
		"kelvin-sign folded key": `{"requests":[{"custom_id":"a","params":{"model":"m","k":1,"\u212a":2}}]}`,
		"case-variant params":    `{"requests":[{"custom_id":"a","params":{"model":"m"},"Params":{"model":"x"}}]}`,
		"duplicate messages":     `{"requests":[{"custom_id":"a","params":{"model":"m","messages":[],"messages":[]}}]}`,
		"duplicate params":       `{"requests":[{"custom_id":"a","params":{"model":"ok"},"params":{"model":"evil"}}]}`,
		"duplicate requests":     `{"requests":[{"custom_id":"a","params":{"model":"ok"}}],"requests":[{"custom_id":"b","params":{"model":"evil"}}]}`,
		"trailing data":          `{"requests":[{"custom_id":"a","params":{"model":"m"}}]} {}`,
		"element not object":     `{"requests":[1]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := splitBatch([]byte(body))
			require.Error(t, err)
			assert.ErrorIs(t, err, errBatchBody)
		})
	}
}

func TestRewriteResultsURL(t *testing.T) {
	obj := map[string]any{"id": "msgbatch_1.t1kab.cd", "results_url": "https://api.anthropic.com/v1/messages/batches/msgbatch_1/results"}
	rewriteResultsURL(obj, nativeapi.TransformEnv{})
	assert.Equal(t, "/v1/messages/batches/msgbatch_1.t1kab.cd/results", obj["results_url"])

	obj = map[string]any{"id": "msgbatch_1.t1kab.cd", "results_url": "https://api.anthropic.com/x"}
	rewriteResultsURL(obj, nativeapi.TransformEnv{PublicBaseURL: "https://gw/llm/rest/c/"})
	assert.Equal(t, "https://gw/llm/rest/c/v1/messages/batches/msgbatch_1.t1kab.cd/results", obj["results_url"])

	obj = map[string]any{"id": "x", "results_url": nil}
	rewriteResultsURL(obj, nativeapi.TransformEnv{})
	assert.Nil(t, obj["results_url"])
}

func TestFoldKeyMatchesEqualFold(t *testing.T) {
	for _, pair := range [][2]string{{"messages", "Messages"}, {"messages", "meſſages"}, {"k", "\u212a"}, {"system", "SYSTEM"}, {"a", "b"}, {"model", "modeI"}} {
		assert.Equal(t, strings.EqualFold(pair[0], pair[1]), foldKey(pair[0]) == foldKey(pair[1]), "%q vs %q", pair[0], pair[1])
	}
}
