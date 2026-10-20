package proxy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/segments"
)

// setMarker puts marker at a responsesTextField path in obj ("summary[].text",
// "action.queries[]"), creating what is missing.
func setMarker(obj map[string]any, path []string, marker string) {
	key, isArray := strings.CutSuffix(path[0], "[]")
	last := len(path) == 1
	switch {
	case last && isArray:
		obj[key] = []any{marker}
	case last:
		obj[key] = marker
	case isArray:
		child := map[string]any{}
		setMarker(child, path[1:], marker)
		obj[key] = []any{child}
	default:
		child, _ := obj[key].(map[string]any)
		if child == nil {
			child = map[string]any{}
			obj[key] = child
		}
		setMarker(child, path[1:], marker)
	}
}

// Every text the proxy's Responses extractor screens is in the detection
// text of some guardrails/segments unit, for every item type the extractor
// knows: a guardrail reading segments (Enterprise scripts' input.segments,
// rewriters' post-filter checks) sees what the request filters see.
func TestResponsesSegmenterSeesWhatTheExtractorScreens(t *testing.T) {
	types := make([]string, 0, len(responsesItemTextFields))
	for typ := range responsesItemTextFields {
		types = append(types, typ)
	}
	sort.Strings(types)

	items := []any{
		map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "MARK-user"}}},
		map[string]any{"type": "message", "role": "developer", "content": "MARK-developer"},
		map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "MARK-assistant"}, map[string]any{"type": "refusal", "refusal": "MARK-refusal"}}},
		map[string]any{"type": "function_call", "name": "f", "call_id": "c1", "arguments": `{"a":"MARK-fn-args"}`},
		map[string]any{"type": "function_call_output", "call_id": "c1", "output": "MARK-fn-output"},
		map[string]any{"type": "custom_tool_call", "name": "p", "call_id": "c2", "input": "MARK-custom-input"},
		map[string]any{"type": "custom_tool_call_output", "call_id": "c2", "output": []any{map[string]any{"type": "input_text", "text": "MARK-custom-output"}}},
	}
	for _, typ := range types {
		item := map[string]any{"type": typ, "id": typ + "_1"}
		for i, f := range responsesItemTextFields[typ] {
			setMarker(item, strings.Split(f.path, "."), fmt.Sprintf("MARK-%s-%d", typ, i))
		}
		if typ == "mcp_list_tools" {
			item["tools"].([]any)[0].(map[string]any)["name"] = "listed"
		}
		items = append(items, item)
	}
	body, err := json.Marshal(map[string]any{"instructions": "MARK-instructions", "input": items,
		"prompt": map[string]any{"id": "pmpt", "variables": map[string]any{"v": "MARK-var"}}})
	if err != nil {
		t.Fatal(err)
	}

	_, slots, err := responsesSlots(body)
	if err != nil {
		t.Fatalf("the proxy extractor refused the body: %v", err)
	}
	units, err := segments.Read(segments.FormatOpenAIResponses, body)
	if err != nil {
		t.Fatal(err)
	}
	var seen strings.Builder
	for _, u := range units {
		seen.WriteString(u.Text)
		seen.WriteString("\n")
	}
	checked := 0
	for _, s := range slots {
		if !strings.Contains(s.text, "MARK-") {
			continue
		}
		// The extractor joins a message's parts with newlines; segments
		// keeps each part.
		for _, part := range strings.Split(s.text, "\n") {
			checked++
			if !strings.Contains(seen.String(), part) {
				t.Errorf("the extractor screens %q (role %s); no segments unit reads it", part, s.role)
			}
		}
	}
	if checked < len(types)+8 {
		t.Fatalf("only %d texts checked", checked)
	}
}
