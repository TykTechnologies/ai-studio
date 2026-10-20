package extract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var requestFormats = []string{FormatAnthropic, FormatOpenAI, FormatOllama, FormatGoogleAI, FormatVertex, FormatBedrock, FormatOpenAIResponses}

type idName struct{ ID, Name string }

// Every in-scope fixture: the trailing tool results (manifest last_user tool
// segments, one per call) equal what ParseRequest extracts, with ids and
// names, in body order.
func TestParseRequest_ToolFixtures(t *testing.T) {
	fixtures := toolfixtures.ByFormat(requestFormats...)
	require.NotEmpty(t, fixtures)
	for _, f := range fixtures {
		if f.Status != "in_scope" {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			var want []idName
			seen := map[idName]bool{}
			for _, s := range f.LastUserSegments() {
				if !s.IsTool() {
					continue
				}
				k := idName{s.ToolCallID, s.ToolName}
				if !seen[k] {
					seen[k] = true
					want = append(want, k)
				}
			}
			r, err := ParseRequest(f.Format, f.Body, testFP)
			require.NoError(t, err)
			var got []idName
			for _, tr := range r.ToolResults {
				got = append(got, idName{tr.ID, tr.Name})
				assert.NotNil(t, tr.ResultFP, "result %s has a fingerprint", tr.ID)
			}
			assert.Equal(t, want, got)

			if len(want) > 0 {
				assert.Equal(t, "tool", r.LastRole)
			}
			for _, s := range f.LastUserSegments() {
				if s.IsTool() && s.IsError {
					assertResultError(t, r, s.ToolCallID)
				}
			}
		})
	}
}

func assertResultError(t *testing.T, r Request, id string) {
	t.Helper()
	for _, tr := range r.ToolResults {
		if tr.ID == id {
			assert.True(t, tr.IsError, "%s is an error result", id)
			return
		}
	}
	t.Errorf("no result %s", id)
}

func TestParseRequest_AnthropicIsErrorAndTurns(t *testing.T) {
	body := `{"model":"claude","messages":[
	  {"role":"user","content":"go"},
	  {"role":"assistant","content":[{"type":"tool_use","id":"toolu_old","name":"a","input":{}}]},
	  {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_old","content":"old result"}]},
	  {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read","input":{}},{"type":"tool_use","id":"toolu_2","name":"write","input":{}}]},
	  {"role":"user","content":[
	    {"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"ok"}]},
	    {"type":"tool_result","tool_use_id":"toolu_2","is_error":true,"content":"permission denied"}]}]}`
	r, err := ParseRequest(FormatAnthropic, []byte(body), testFP)
	require.NoError(t, err)
	require.Len(t, r.ToolResults, 2, "the history result is not trailing")
	assert.Equal(t, idName{"toolu_1", "read"}, idName{r.ToolResults[0].ID, r.ToolResults[0].Name})
	assert.False(t, r.ToolResults[0].IsError)
	assert.True(t, r.ToolResults[1].IsError)
	assert.Equal(t, ResultFingerprint(testFP, "permission denied"), r.ToolResults[1].ResultFP)

	var x agenttrace.Extracts
	r.Apply(&x)
	require.Len(t, x.ToolResults, 2)
	assert.True(t, x.ToolResults[1].IsError)
}

func TestParseRequest_NewUserTurn(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"next"}]}`
	r, err := ParseRequest(FormatOpenAI, []byte(body), testFP)
	require.NoError(t, err)
	assert.Empty(t, r.ToolResults)
	assert.Equal(t, "user", r.LastRole)
}

func TestParseRequest_Responses(t *testing.T) {
	body := `{"previous_response_id":"resp_prev","input":[
	  {"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
	  {"type":"function_call_output","call_id":"call_1","output":"42"}]}`
	r, err := ParseRequest(FormatOpenAIResponses, []byte(body), testFP)
	require.NoError(t, err)
	assert.Equal(t, "resp_prev", r.PreviousResponseID)
	require.Len(t, r.ToolResults, 1)
	assert.Equal(t, idName{"call_1", "lookup"}, idName{r.ToolResults[0].ID, r.ToolResults[0].Name})
	assert.Equal(t, "tool", r.LastRole)

	r, err = ParseRequest(FormatOpenAIResponses, []byte(`{"input":"just text","previous_response_id":"resp_9"}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "user", r.LastRole)
	assert.Equal(t, "resp_9", r.PreviousResponseID)
}

// Replayed server tool outputs (mcp_call) are reported but never become R1
// results in the event.
func TestRequestApply_SkipsServerAndIDless(t *testing.T) {
	r := Request{ToolResults: []ToolResult{{ID: "a"}, {ID: "", Name: "get_weather"}, {ID: "mcp_1", Server: true}}}
	var x agenttrace.Extracts
	r.Apply(&x)
	require.Len(t, x.ToolResults, 1)
	assert.Equal(t, "a", x.ToolResults[0].ID)
}

func TestParseRequest_Errors(t *testing.T) {
	_, err := ParseRequest("cohere", []byte(`{}`), nil)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
	_, err = ParseRequest(FormatOpenAIResponses, []byte(`not json`), nil)
	assert.ErrorIs(t, err, ErrUnrecognisedBody)
	_, err = ParseRequest(FormatAnthropic, []byte(`not json`), nil)
	assert.Error(t, err)
}

func TestParseResponse_Formats(t *testing.T) {
	cases := []struct {
		name, format, body string
		id                 string
		calls              []idName
		server             []idName
		finish             []string
		usage              bool
	}{
		{
			name: "openai chat", format: FormatOpenAI,
			body: `{"id":"chatcmpl-1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[
				{"id":"call_a","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}},
				{"id":"call_b","type":"function","function":{"name":"fetch","arguments":"{}"}}]}}],"usage":{"total_tokens":3}}`,
			id: "chatcmpl-1", calls: []idName{{"call_a", "search"}, {"call_b", "fetch"}}, finish: []string{"tool_calls"}, usage: true,
		},
		{
			name: "openai legacy function_call", format: FormatOpenAI,
			body: `{"id":"c2","choices":[{"finish_reason":"function_call","message":{"function_call":{"name":"f","arguments":"{}"}}}]}`,
			id:   "c2", calls: []idName{{"", "f"}}, finish: []string{"function_call"},
		},
		{
			name: "ollama", format: FormatOllama,
			body:  `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"Oslo"}}}]},"done_reason":"stop","eval_count":5}`,
			calls: []idName{{"", "get_weather"}}, finish: []string{"stop"}, usage: true,
		},
		{
			name: "anthropic with server tools", format: FormatAnthropic,
			body: `{"id":"msg_1","stop_reason":"tool_use","content":[
				{"type":"text","text":"Let me check."},
				{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"x"}},
				{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[]},
				{"type":"thinking","thinking":"...","signature":"sig"},
				{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"README.md"}}],"usage":{"input_tokens":1}}`,
			id: "msg_1", calls: []idName{{"toolu_1", "read_file"}}, server: []idName{{"srvtoolu_1", "web_search"}}, finish: []string{"tool_use"}, usage: true,
		},
		{
			name: "gemini", format: FormatGoogleAI,
			body: `{"responseId":"r1","candidates":[{"finishReason":"STOP","content":{"parts":[
				{"functionCall":{"id":"fc-1","name":"web_search","args":{"query":"x"}},"thoughtSignature":"abc"},
				{"function_call":{"name":"get_time","args":{}}}]}}],"usageMetadata":{"totalTokenCount":2}}`,
			id: "r1", calls: []idName{{"fc-1", "web_search"}, {"", "get_time"}}, finish: []string{"STOP"}, usage: true,
		},
		{
			name: "bedrock converse", format: FormatBedrock,
			body:  `{"output":{"message":{"role":"assistant","content":[{"text":"ok"},{"toolUse":{"toolUseId":"tooluse_1","name":"fetch_url","input":{"url":"https://x"}}}]}},"stopReason":"tool_use","usage":{"totalTokens":4}}`,
			calls: []idName{{"tooluse_1", "fetch_url"}}, finish: []string{"tool_use"}, usage: true,
		},
		{
			name: "bedrock invoke anthropic body", format: FormatBedrock,
			body: `{"id":"msg_b","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_b","name":"t","input":{}}]}`,
			id:   "msg_b", calls: []idName{{"toolu_b", "t"}}, finish: []string{"tool_use"},
		},
		{
			name: "responses", format: FormatOpenAIResponses,
			body: `{"id":"resp_1","status":"completed","output":[
				{"type":"reasoning","id":"rs_1","encrypted_content":"opaque"},
				{"type":"web_search_call","id":"ws_1","status":"completed"},
				{"type":"mcp_call","id":"mcp_1","name":"find_contact","server_label":"crm","arguments":"{\"name\":\"Sam\"}","output":"Sam"},
				{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"k\":1}"},
				{"type":"custom_tool_call","id":"ctc_1","call_id":"call_2","name":"grammar","input":"teh"}],"usage":{"total_tokens":9}}`,
			id: "resp_1", calls: []idName{{"call_1", "lookup"}, {"call_2", "grammar"}},
			server: []idName{{"ws_1", "web_search"}, {"mcp_1", "find_contact"}}, finish: []string{"completed"}, usage: true,
		},
		{
			name: "responses body under openai", format: FormatOpenAI,
			body: `{"id":"resp_2","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","call_id":"call_9","name":"x","arguments":"{"}]}`,
			id:   "resp_2", calls: []idName{{"call_9", "x"}}, finish: []string{"incomplete", "max_output_tokens"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := ParseResponse(c.format, []byte(c.body), testFP)
			require.NoError(t, err)
			assert.Equal(t, c.id, r.ID)
			assert.Equal(t, c.calls, idNames(r.ToolCalls))
			assert.Equal(t, c.server, idNames(r.ServerToolCalls))
			assert.Equal(t, c.finish, r.FinishReasons)
			assert.Equal(t, c.usage, r.HasUsage)
		})
	}
}

func idNames(calls []ToolCall) []idName {
	var out []idName
	for _, c := range calls {
		out = append(out, idName{c.ID, c.Name})
	}
	return out
}

// R1/R2 hinge on this: the model's arguments in a response fingerprint equal
// to the same call executed through MCP, across vendors and shapes.
func TestParseResponse_ArgsMatchExecutedCall(t *testing.T) {
	executed, err := ArgsFingerprint(testFP, []byte(`{"body_name":"Sam","id":"7"}`))
	require.NoError(t, err)

	openai := `{"choices":[{"message":{"tool_calls":[{"id":"c","function":{"name":"x","arguments":"{\"id\":\"7\",\"body\":{\"name\":\"Sam\"}}"}}]}}]}`
	anthropic := `{"content":[{"type":"tool_use","id":"t","name":"x","input":{"body":{"name":"Sam"},"id":"7"}}]}`
	for format, body := range map[string]string{FormatOpenAI: openai, FormatAnthropic: anthropic} {
		r, err := ParseResponse(format, []byte(body), testFP)
		require.NoError(t, err)
		require.Len(t, r.ToolCalls, 1)
		assert.Equal(t, executed, r.ToolCalls[0].ArgsFP, format)
	}
}

// Malformed arguments keep the id and name and drop only the fingerprint.
func TestParseResponse_MalformedArgs(t *testing.T) {
	r, err := ParseResponse(FormatOpenAI, []byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","function":{"name":"x","arguments":"{\"a\":"}}]}}]}`), testFP)
	require.NoError(t, err)
	require.Len(t, r.ToolCalls, 1)
	assert.Equal(t, "c1", r.ToolCalls[0].ID)
	assert.Nil(t, r.ToolCalls[0].ArgsFP)

	var x agenttrace.Extracts
	r.Apply(&x)
	require.Len(t, x.ToolCalls, 1)
	assert.Equal(t, "c1", x.ToolCalls[0].ID)
}

func TestParseResponse_Errors(t *testing.T) {
	_, err := ParseResponse("cohere", []byte(`{}`), nil)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
	_, err = ParseResponse(FormatAnthropic, []byte(`[1,2]`), nil)
	assert.ErrorIs(t, err, ErrUnrecognisedBody)
}

// Nothing raw is kept: no string in any extract (request or response, every
// fixture) holds a piece of argument or result content. The extracts carry
// only ids, names, roles, finish reasons and fingerprints, so a content mode
// cannot change them and off mode retains nothing.
func TestExtracts_RetainNoContent(t *testing.T) {
	secret := "s3cr3t-content-canary"
	bodies := map[string]string{
		FormatOpenAI:          `{"choices":[{"message":{"tool_calls":[{"id":"c","function":{"name":"x","arguments":"{\"q\":\"` + secret + `\"}"}}]}}]}`,
		FormatAnthropic:       `{"content":[{"type":"tool_use","id":"t","name":"x","input":{"q":"` + secret + `"}}]}`,
		FormatOpenAIResponses: `{"output":[{"type":"mcp_call","id":"m","name":"x","arguments":"{\"q\":\"` + secret + `\"}","output":"` + secret + `"}]}`,
	}
	for format, body := range bodies {
		r, err := ParseResponse(format, []byte(body), testFP)
		require.NoError(t, err)
		assertNoString(t, r, secret)
	}
	reqs := map[string]string{
		FormatAnthropic:       `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"x","input":{"q":"` + secret + `"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"` + secret + `"}]}]}`,
		FormatOpenAIResponses: `{"input":[{"type":"function_call_output","call_id":"c","output":"` + secret + `"}]}`,
	}
	for format, body := range reqs {
		r, err := ParseRequest(format, []byte(body), testFP)
		require.NoError(t, err)
		require.NotEmpty(t, r.ToolResults)
		assertNoString(t, r, secret)
	}
	for _, f := range toolfixtures.ByFormat(requestFormats...) {
		r, err := ParseRequest(f.Format, f.Body, testFP)
		require.NoError(t, err)
		for _, marker := range []string{"[canary:", "[marker:", toolfixturesPII} {
			assertNoString(t, r, marker)
		}
	}
}

const toolfixturesPII = "jane.doe@example.com"

// assertNoString walks v's string fields (not fingerprints) for needle.
func assertNoString(t *testing.T, v any, needle string) {
	t.Helper()
	var walk func(reflect.Value, string)
	walk = func(rv reflect.Value, path string) {
		switch rv.Kind() {
		case reflect.String:
			assert.NotContains(t, rv.String(), needle, path)
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				walk(rv.Field(i), path+"."+rv.Type().Field(i).Name)
			}
		case reflect.Slice:
			if rv.Type().Elem().Kind() == reflect.Uint8 {
				assert.NotContains(t, string(rv.Bytes()), needle, path)
				return
			}
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(reflect.ValueOf(v), reflect.TypeOf(v).Name())
}

// bigAnthropicRequest is a ~32 KiB agent-loop request: history, a system
// prompt and three trailing tool results.
func bigAnthropicRequest() []byte {
	var b strings.Builder
	b.WriteString(`{"model":"claude","system":"` + strings.Repeat("You are a careful agent. ", 40) + `","messages":[`)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, `{"role":"user","content":"%s"},`, strings.Repeat("please look into item ", 20))
		fmt.Fprintf(&b, `{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_h%02d","name":"read","input":{"path":"/a/%d","depth":2}}]},`, i, i)
		fmt.Fprintf(&b, `{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_h%02d","content":"%s"}]},`, i, strings.Repeat("line of history output ", 60))
	}
	b.WriteString(`{"role":"assistant","content":[`)
	ids := []string{}
	for i := 0; i < 3; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		ids = append(ids, fmt.Sprintf("toolu_c%d", i))
		fmt.Fprintf(&b, `{"type":"tool_use","id":"toolu_c%d","name":"grep","input":{"q":"x%d"}}`, i, i)
	}
	b.WriteString(`]},{"role":"user","content":[`)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"type":"tool_result","tool_use_id":"%s","content":"%s"}`, id, strings.Repeat("match found here ", 60))
	}
	b.WriteString(`]}]}`)
	return []byte(b.String())
}

func bigOpenAIResponse() []byte {
	var b strings.Builder
	b.WriteString(`{"id":"chatcmpl-big","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"` + strings.Repeat("reasoning text ", 2100) + `","tool_calls":[`)
	for i := 0; i < 4; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"call_%d","type":"function","function":{"name":"search","arguments":"{\"q\":\"%s\",\"n\":%d}"}}`, i, strings.Repeat("term ", 50), i)
	}
	b.WriteString(`]}}],"usage":{"total_tokens":10}}`)
	return []byte(b.String())
}

func TestBenchBodiesAreRealistic(t *testing.T) {
	req, resp := bigAnthropicRequest(), bigOpenAIResponse()
	assert.InDelta(t, 32*1024, len(req), 8*1024, "request ~32 KiB, got %d", len(req))
	assert.InDelta(t, 32*1024, len(resp), 8*1024, "response ~32 KiB, got %d", len(resp))
	r, err := ParseRequest(FormatAnthropic, req, testFP)
	require.NoError(t, err)
	got := make([]string, 0, len(r.ToolResults))
	for _, tr := range r.ToolResults {
		got = append(got, tr.ID)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"toolu_c0", "toolu_c1", "toolu_c2"}, got)
}

// Benchmarks for the §10 budget (bead .14 records the numbers).
func BenchmarkParseRequest_Anthropic32KiB(b *testing.B) {
	body := bigAnthropicRequest()
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseRequest(FormatAnthropic, body, testFP); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseResponse_OpenAI32KiB(b *testing.B) {
	body := bigOpenAIResponse()
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseResponse(FormatOpenAI, body, testFP); err != nil {
			b.Fatal(err)
		}
	}
}
