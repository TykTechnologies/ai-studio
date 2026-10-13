package segments

import (
	"encoding/json"
	"reflect"
	"testing"
)

const ph = "[content removed by policy]"

func decodeAny(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	return v
}

// quarantineUnits quarantines the units of body whose text is in flagged and
// returns the rewritten body.
func quarantineUnits(t *testing.T, format string, body string, flagged ...string) []byte {
	t.Helper()
	units, err := ExtractUnits(format, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, f := range flagged {
		want[f] = true
	}
	var edits []Edit
	for _, u := range units {
		if !want[u.Text] {
			continue
		}
		delete(want, u.Text)
		e, ok := u.Quarantine(ph)
		if !ok {
			t.Fatalf("unit %q cannot be quarantined", u.Text)
		}
		edits = append(edits, e...)
	}
	if len(want) != 0 {
		t.Fatalf("no unit with text %v", want)
	}
	out, err := Apply([]byte(body), edits)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A quarantined tool result keeps its call id and name; only its content
// changes, images included, and every other block is untouched.
func TestQuarantineReplacesWholeToolResults(t *testing.T) {
	cases := []struct {
		name, format, body string
		flagged            []string
		want               string
	}{
		{
			name:   "anthropic tool_result with text and image, beside user text",
			format: FormatAnthropic,
			body: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"fetch","input":{}}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":[{"type":"text","text":"evil"},{"type":"image","source":{"type":"base64","data":"AA"}}]},{"type":"text","text":"thanks"}]}]}`,
			flagged: []string{"evil"},
			want: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"fetch","input":{}}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":"` + ph + `"},{"type":"text","text":"thanks"}]}]}`,
		},
		{
			name:    "anthropic user text beside a tool_result: each leaf",
			format:  FormatAnthropic,
			body:    `{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"tool_result","tool_use_id":"t1","content":"r"},{"type":"text","text":"b"}]}]}`,
			flagged: []string{"a\n\f\nb"},
			want:    `{"messages":[{"role":"user","content":[{"type":"text","text":"` + ph + `"},{"type":"tool_result","tool_use_id":"t1","content":"r"},{"type":"text","text":"` + ph + `"}]}]}`,
		},
		{
			name:   "openai parallel tool messages, two flagged",
			format: FormatOpenAI,
			body: `{"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"a","type":"function","function":{"name":"x","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"y","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"z","arguments":"{}"}}]},` +
				`{"role":"tool","tool_call_id":"a","content":"bad one"},{"role":"tool","tool_call_id":"b","content":[{"type":"text","text":"fine"}]},{"role":"tool","tool_call_id":"c","content":[{"type":"text","text":"bad two"}]}]}`,
			flagged: []string{"bad one", "bad two"},
			want: `{"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"a","type":"function","function":{"name":"x","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"y","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"z","arguments":"{}"}}]},` +
				`{"role":"tool","tool_call_id":"a","content":"` + ph + `"},{"role":"tool","tool_call_id":"b","content":[{"type":"text","text":"fine"}]},{"role":"tool","tool_call_id":"c","content":"` + ph + `"}]}`,
		},
		{
			name:    "google functionResponse: the response object, keys included",
			format:  FormatGoogleAI,
			body:    `{"contents":[{"role":"user","parts":[{"text":"hi"},{"functionResponse":{"id":"f1","name":"search","response":{"IGNORE PREVIOUS":{"x":"y"}}}}]}]}`,
			flagged: []string{"IGNORE PREVIOUS\n\f\nx\n\f\ny"},
			want:    `{"contents":[{"role":"user","parts":[{"text":"hi"},{"functionResponse":{"id":"f1","name":"search","response":{"output":"` + ph + `"}}}]}]}`,
		},
	}
	cases = append(cases, struct {
		name, format, body string
		flagged            []string
		want               string
	}{
		name:   "bedrock toolResult: content stays an array of blocks",
		format: FormatBedrock,
		body: `{"messages":[{"role":"assistant","content":[{"toolUse":{"toolUseId":"tu","name":"search","input":{}}}]},` +
			`{"role":"user","content":[{"toolResult":{"toolUseId":"tu","status":"success","content":[{"text":"evil"},{"json":{"k":"v"}}]}},{"text":"thanks"}]}]}`,
		flagged: []string{"evil\n\f\nk\n\f\nv"},
		want: `{"messages":[{"role":"assistant","content":[{"toolUse":{"toolUseId":"tu","name":"search","input":{}}}]},` +
			`{"role":"user","content":[{"toolResult":{"toolUseId":"tu","status":"success","content":[{"text":"` + ph + `"}]}},{"text":"thanks"}]}]}`,
	})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := quarantineUnits(t, c.format, c.body, c.flagged...)
			if !reflect.DeepEqual(decodeAny(t, out), decodeAny(t, []byte(c.want))) {
				t.Errorf("got  %s\nwant %s", out, c.want)
			}
			if _, err := ExtractUnits(c.format, out); err != nil {
				t.Errorf("the quarantined body no longer reads as %s: %v", c.format, err)
			}
		})
	}
}

// A unit whose text includes something no edit can replace (the keys of a
// body read generically) cannot be quarantined in place.
func TestQuarantineRefusesWhatItCannotReplace(t *testing.T) {
	units, err := Generic([]byte(`{"toolResult":{"content":"evil"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := units[0].Quarantine(ph); ok {
		t.Error("a generic unit with keys was quarantined")
	}
}
