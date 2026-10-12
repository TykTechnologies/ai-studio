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
		{http.MethodPost, "/v1/messages/count_tokens", "anthropic.messages.count_tokens"},
	} {
		got, ok := m.Match(tc.method, tc.path)
		require.True(t, ok, tc.path)
		assert.Equal(t, tc.op, got.Op.Name)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/messages"},
		{http.MethodGet, "/v1/messages/count_tokens"},
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
		"nested duplicate content": `{"requests":[{"custom_id":"a","params":{"model":"m",` +
			`"messages":[{"role":"user","content":"ok","content":"evil"}]}}]}`,
		"nested case-variant content": `{"requests":[{"custom_id":"a","params":{"model":"m",` +
			`"messages":[{"role":"user","content":"ok","Content":"evil"}]}}]}`,
		"nested duplicate in system": `{"requests":[{"custom_id":"a","params":{"model":"m",` +
			`"system":[{"type":"text","text":"a","text":"b"}],"messages":[]}}]}`,
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

func TestSplitBatchErrorNamesTheBodyOnce(t *testing.T) {
	_, err := splitBatch([]byte(`{"requests":[{"custom_id":"a","params":{"model":"m","messages":[],"Messages":[]}}]}`))
	require.Error(t, err)
	assert.Equal(t, `invalid message batch body: request 0: params: duplicate key "Messages"`, err.Error())
	assert.ErrorIs(t, err, errBatchBody)

	_, err = splitBatch([]byte(`{"requests":[{"custom_id":"a","params":{"model":"m"}}],"Requests":[]}`))
	require.Error(t, err)
	assert.Equal(t, `invalid message batch body: duplicate key "Requests"`, err.Error())
}

func TestSplitCountTokens(t *testing.T) {
	body := `{"model":"claude-luna","messages":[{"role":"user","content":"hi"}],"system":"s"}`
	items, err := splitCountTokens([]byte(body))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, nativeapi.Item{Format: "anthropic", Model: "claude-luna", Body: []byte(body)}, items[0])

	items[0].Body = []byte(`{"model":"claude-luna","messages":[]}`)
	out, err := joinSingle([]byte(body), items)
	require.NoError(t, err)
	assert.Equal(t, `{"model":"claude-luna","messages":[]}`, string(out))
	_, err = joinSingle([]byte(body), nil)
	assert.Error(t, err)

	for name, b := range map[string]string{
		"not json":         `nope`,
		"not an object":    `[]`,
		"no model":         `{"messages":[]}`,
		"model not string": `{"model":1,"messages":[]}`,
		"duplicate model":  `{"model":"ok","model":"evil","messages":[]}`,
		"case-variant":     `{"model":"m","messages":[],"Messages":[]}`,
		"nested duplicate": `{"model":"m","messages":[{"role":"user","content":"a","content":"b"}]}`,
		"trailing data":    `{"model":"m"} {}`,
	} {
		_, err := splitCountTokens([]byte(b))
		assert.ErrorIs(t, err, errCountTokensBody, name)
	}
	_, err = splitCountTokens([]byte(`{"model":"m","Model":"x"}`))
	assert.EqualError(t, err, `invalid count_tokens body: duplicate key "Model"`)
}

// Case variants in free-form subtrees (a tool's schema, a tool call's input)
// are different keys to everyone; only exact duplicates are refused there.
func TestSplitBatchAcceptsFreeFormCaseVariants(t *testing.T) {
	body := `{"requests":[{"custom_id":"a","params":{"model":"m","max_tokens":5,` +
		`"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"id":{"type":"string"},"ID":{"type":"string"}}}}],` +
		`"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"lookup","input":{"Name":"a","name":"b"}}]}]}}]}`
	items, err := splitBatch([]byte(body))
	require.NoError(t, err)
	require.Len(t, items, 1)
	_, err = splitCountTokens([]byte(`{"model":"m","messages":[],"tools":[{"name":"t","input_schema":{"properties":{"id":{},"ID":{}}}}]}`))
	require.NoError(t, err)
}
