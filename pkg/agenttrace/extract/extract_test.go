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
			if w, ok := responsesTrailing[f.Name]; ok {
				// The manifest's last_user is "after the last assistant
				// message" (guardrail scope); correlation's trailing is
				// "after the last item the model produced" (jyx.61).
				r, err := ParseRequest(f.Format, f.Body, testFP)
				require.NoError(t, err)
				var got []idName
				for _, tr := range r.ToolResults {
					got = append(got, idName{tr.ID, tr.Name})
				}
				assert.Equal(t, w, got)
				return
			}
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

// responsesTrailing are the trailing results of the Responses fixtures. In
// mixed_outputs the function_call_output for call_R2build is followed by
// another model call, so it is history.
var responsesTrailing = map[string][]idName{
	"openai_responses_function_call_output": {{"call_R1fetch0000000000000001", "fetch_url"}},
	"openai_responses_mixed_outputs":        {{"call_R2gram00000000000000001", "grammar_check"}, {"mcp_R2crm000000000000000001", "find_contact"}},
	"openai_responses_server_tools":         nil,
}

// jyx.61: a Responses tool loop with no assistant message between calls
// resends earlier outputs as history; only the outputs after the model's
// last call are trailing.
func TestParseRequest_ResponsesToolLoopHistory(t *testing.T) {
	ids := func(body string) []string {
		r, err := ParseRequest(FormatOpenAIResponses, []byte(body), nil)
		require.NoError(t, err)
		var out []string
		for _, tr := range r.ToolResults {
			out = append(out, tr.ID)
		}
		return out
	}
	user := `{"role":"user","content":"go"}`
	fc := func(id string) string {
		return `{"type":"function_call","call_id":"` + id + `","name":"f","arguments":"{}"}`
	}
	out := func(id string) string { return `{"type":"function_call_output","call_id":"` + id + `","output":"r"}` }

	sequential := `{"input":[` + user + `,` + fc("c1") + `,` + out("c1") + `,` + fc("c2") + `,` + out("c2") + `]}`
	assert.Equal(t, []string{"c2"}, ids(sequential), "c1 is history")

	parallel := `{"input":[` + user + `,` + fc("c1") + `,` + fc("c2") + `,` + out("c1") + `,` + out("c2") + `]}`
	assert.Equal(t, []string{"c1", "c2"}, ids(parallel))

	reasoning := `{"input":[` + user + `,` + fc("c1") + `,` + out("c1") + `,{"type":"reasoning","id":"rs","summary":[]},` + fc("c2") + `,` + out("c2") + `]}`
	assert.Equal(t, []string{"c2"}, ids(reasoning))

	// Client-executed computer and shell calls are answered like functions.
	computer := `{"input":[` + user + `,{"type":"computer_call","call_id":"cc1","action":{"type":"click"}},` +
		`{"type":"computer_call_output","call_id":"cc1","output":{"type":"computer_screenshot","image_url":"data:image/png;base64,AA"}},` +
		`{"type":"local_shell_call","call_id":"sh1","action":{"type":"exec"}},{"type":"local_shell_call_output","call_id":"sh1","output":"ok"}]}`
	r, err := ParseRequest(FormatOpenAIResponses, []byte(computer), testFP)
	require.NoError(t, err)
	require.Len(t, r.ToolResults, 1, "the computer call is history once the shell call follows")
	assert.Equal(t, idName{"sh1", "local_shell_call"}, idName{r.ToolResults[0].ID, r.ToolResults[0].Name})
	assert.NotNil(t, r.ToolResults[0].ResultFP)

	newTurn := `{"input":[` + user + `,` + fc("c1") + `,` + out("c1") + `,{"role":"assistant","content":"done"},` + user + `]}`
	assert.Empty(t, ids(newTurn))
}

// jyx.62: untrusted bodies cannot inflate the extracts.
func TestParse_Bounds(t *testing.T) {
	long := strings.Repeat("x", 3<<20)
	var outs []string
	for i := 0; i < 2*MaxToolCalls; i++ {
		outs = append(outs, fmt.Sprintf(`{"type":"function_call_output","call_id":"c%d","output":"r"}`, i))
	}
	body := `{"previous_response_id":"` + long + `","input":[{"type":"function_call","call_id":"` + long + `","name":"` + long + `","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"` + long + `","output":"x"},` + strings.Join(outs, ",") + `,{"role":"wizard","content":"hi"}]}`
	r, err := ParseRequest(FormatOpenAIResponses, []byte(body), nil)
	require.NoError(t, err)
	assert.Empty(t, r.PreviousResponseID, "an oversized id is dropped, not cut")
	assert.Len(t, r.ToolResults, MaxToolCalls)
	assert.Equal(t, fmt.Sprintf("c%d", 2*MaxToolCalls-1), r.ToolResults[MaxToolCalls-1].ID, "the latest results are kept")
	assert.Equal(t, "other", r.LastRole)

	var calls []string
	for i := 0; i < 2*MaxToolCalls; i++ {
		calls = append(calls, fmt.Sprintf(`{"id":"c%d","function":{"name":"f","arguments":"{}"}}`, i))
	}
	calls = append(calls, `{"id":"`+long+`","function":{"name":"`+long+`","arguments":"{}"}}`)
	resp := `{"id":"` + long + `","choices":[{"finish_reason":"` + long + `","message":{"tool_calls":[` + strings.Join(calls, ",") + `]}}]}`
	rr, err := ParseResponse(FormatOpenAI, []byte(resp), nil)
	require.NoError(t, err)
	assert.Empty(t, rr.ID)
	assert.Len(t, rr.ToolCalls, MaxToolCalls)
	require.Len(t, rr.FinishReasons, 1)
	assert.LessOrEqual(t, len(rr.FinishReasons[0]), 64)

	small := `{"choices":[{"message":{"tool_calls":[{"id":"` + long[:300] + `","function":{"name":"` + long[:300] + `","arguments":"{}"}}]}}]}`
	rr, err = ParseResponse(FormatOpenAI, []byte(small), nil)
	require.NoError(t, err)
	assert.Empty(t, rr.ToolCalls[0].ID)
	assert.Len(t, rr.ToolCalls[0].Name, MaxNameBytes)
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

// A result sent back without its id takes the id of the call it answers,
// by name in order, from the model's last message; a call already answered
// by id is not claimed twice, and an earlier message's call is never used.
func TestParseRequest_ResultIDsFromCalls(t *testing.T) {
	body := `{"contents":[
	  {"role":"user","parts":[{"text":"who reviews?"}]},
	  {"role":"model","parts":[{"functionCall":{"name":"search","args":{},"id":"old_1"}}]},
	  {"role":"user","parts":[{"functionResponse":{"name":"search","response":{"r":0}}}]},
	  {"role":"model","parts":[
	    {"functionCall":{"name":"search","args":{"q":"a"},"id":"call_1"},"thoughtSignature":"x"},
	    {"functionCall":{"name":"read","args":{},"id":"call_2"}},
	    {"functionCall":{"name":"search","args":{"q":"b"},"id":"call_3"}}]},
	  {"role":"user","parts":[
	    {"functionResponse":{"name":"search","id":"call_1","response":{"r":1}}},
	    {"functionResponse":{"name":"search","response":{"r":3}}},
	    {"functionResponse":{"name":"read","response":{"r":2}}},
	    {"functionResponse":{"name":"write","response":{"r":4}}}]}]}`
	for _, f := range []string{FormatGoogleAI, FormatVertex} {
		r, err := ParseRequest(f, []byte(body), testFP)
		require.NoError(t, err)
		var got []idName
		for _, tr := range r.ToolResults {
			got = append(got, idName{tr.ID, tr.Name})
		}
		assert.Equal(t, []idName{{"call_1", "search"}, {"call_3", "search"}, {"call_2", "read"}, {"", "write"}}, got, f)
		assert.Nil(t, r.ToolResults[3].CallArgsFP, "no call to pair with")
		assert.Equal(t, "tool", r.LastRole)
	}
}

// When neither the call nor the result keeps an id (Gemini through clients
// that drop it, Ollama), the result carries the call's name and argument
// fingerprint, equal to the fingerprint the response's call got, whatever
// the key order of the history's copy. A result without a name takes the
// next call in order.
func TestParseRequest_IDlessResultsCarryCallArgs(t *testing.T) {
	cases := []struct {
		format, response, request string
	}{
		{FormatGoogleAI,
			`{"candidates":[{"content":{"role":"model","parts":[
			  {"functionCall":{"name":"search","args":{"q":"a","page":2}}},
			  {"functionCall":{"name":"search","args":{"q":"b"}}}]}}]}`,
			`{"contents":[{"role":"user","parts":[{"text":"go"}]},
			  {"role":"model","parts":[
			    {"functionCall":{"name":"search","args":{"page":2,"q":"a"}},"thoughtSignature":"x"},
			    {"functionCall":{"name":"search","args":{"q":"b"}}}]},
			  {"role":"user","parts":[
			    {"functionResponse":{"name":"search","response":{"r":1}}},
			    {"functionResponse":{"name":"search","response":{"r":2}}}]}]}`},
		{FormatVertex,
			`{"candidates":[{"content":{"role":"model","parts":[
			  {"functionCall":{"name":"search","args":{"q":"a","page":2}}},
			  {"functionCall":{"name":"search","args":{"q":"b"}}}]}}]}`,
			`{"contents":[{"role":"user","parts":[{"text":"go"}]},
			  {"role":"model","parts":[
			    {"functionCall":{"name":"search","args":{"page":2,"q":"a"}}},
			    {"functionCall":{"name":"search","args":{"q":"b"}}}]},
			  {"role":"user","parts":[
			    {"functionResponse":{"name":"search","response":{"r":1}}},
			    {"functionResponse":{"name":"search","response":{"r":2}}}]}]}`},
		{FormatOllama,
			`{"message":{"role":"assistant","content":"","tool_calls":[
			  {"function":{"name":"search","arguments":{"q":"a","page":2}}},
			  {"function":{"name":"search","arguments":{"q":"b"}}}]},"done":true}`,
			`{"messages":[{"role":"user","content":"go"},
			  {"role":"assistant","content":"","tool_calls":[
			    {"function":{"name":"search","arguments":{"page":2,"q":"a"}}},
			    {"function":{"name":"search","arguments":{"q":"b"}}}]},
			  {"role":"tool","tool_name":"search","content":"1"},
			  {"role":"tool","content":"2"}]}`},
	}
	for _, c := range cases {
		resp, err := ParseResponse(c.format, []byte(c.response), testFP)
		require.NoError(t, err, c.format)
		require.Len(t, resp.ToolCalls, 2, c.format)
		req, err := ParseRequest(c.format, []byte(c.request), testFP)
		require.NoError(t, err, c.format)
		require.Len(t, req.ToolResults, 2, c.format)
		for i, tr := range req.ToolResults {
			assert.Empty(t, tr.ID, c.format)
			assert.Equal(t, "search", tr.Name, c.format)
			require.NotNil(t, tr.CallArgsFP, c.format)
			assert.Equal(t, resp.ToolCalls[i].ArgsFP, tr.CallArgsFP, "%s result %d", c.format, i)
		}
		assert.NotEqual(t, req.ToolResults[0].CallArgsFP, req.ToolResults[1].CallArgsFP, c.format)
	}
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
// results in the event. A result without an id is kept with its call's
// name and argument fingerprint; one with an id needs neither.
func TestRequestApply_SkipsServerKeepsIDless(t *testing.T) {
	fp := agenttrace.Fingerprint{1, 2}
	r := Request{ToolResults: []ToolResult{{ID: "a", Name: "lookup", CallArgsFP: fp}, {ID: "", Name: "get_weather", CallArgsFP: fp}, {ID: "mcp_1", Server: true}}}
	var x agenttrace.Extracts
	r.Apply(&x)
	assert.Equal(t, []agenttrace.ToolResult{{ID: "a"}, {Name: "get_weather", CallArgsFP: fp}}, x.ToolResults)
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

// rawAt reads a value in place: through objects and arrays, past strings
// holding brackets and escaped quotes, and not past a missing key or index.
func TestRawAt(t *testing.T) {
	body := []byte(` {"a":"x]}\"y","contents":[{"parts":[{"text":"[{"},{"functionCall":{"name":"s","args":{"q":"a,b","n":[1,2]}}}]},
	  {"parts":[{"functionCall":{"args":{}}}]}], "a/b":{"~":true}} `)
	for ptr, want := range map[string]string{
		"/contents/0/parts/1/functionCall/args":   `{"q":"a,b","n":[1,2]}`,
		"/contents/0/parts/1/functionCall/args/n": `[1,2]`,
		"/contents/1/parts/0/functionCall/args":   `{}`,
		"/a":                                      `"x]}\"y"`,
		"/a~1b/~0":                                `true`,
	} {
		got, ok := rawAt(body, ptr)
		assert.True(t, ok, ptr)
		assert.Equal(t, want, string(got), ptr)
	}
	for _, ptr := range []string{"/missing", "/contents/2", "/contents/0/parts/x", "/a/0", "/contents/-1"} {
		_, ok := rawAt(body, ptr)
		assert.False(t, ok, ptr)
	}
}
