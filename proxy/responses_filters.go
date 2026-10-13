package proxy

// Request filters over OpenAI Responses API bodies (POST /v1/responses,
// /v1/responses/input_tokens, /v1/responses/compact, and batch lines with url
// /v1/responses). These carry no messages array: the prompt is in
// "instructions" (a system message) and "input" (a string, or an array of
// items: messages with text parts, tool call outputs, tool calls, and the
// text fields of reasoning, MCP, code interpreter and other built-in tool
// items), and in the variables of a stored "prompt". Filters and guardrails see each of
// those as one message, in body order; a filter that rewrites messages has
// each changed text written back into the field it came from.
//
// Unlike the Chat Completions extractor, this one fails closed: a body whose
// prompt fields have a shape it cannot read, or an input item of a type it
// does not know, refuses the request when the LLM has request filters,
// rather than screening an empty prompt.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// requestFilterFormatOpenAIResponses is the filter format of a Responses API
// body, keyed like vendor formats in the extractor and reconstructor
// registries.
const requestFilterFormatOpenAIResponses = "openai_responses"

// responsesSlot is one prompt text of a Responses body: its role as filters
// see it, its text, and how to write a rewritten text back.
type responsesSlot struct {
	role llms.ChatMessageType
	text string
	set  func(string)
	// tool names the tool behind a tool result.
	tool scripting.MessageTool
	// owner is the object holding the text, for an item's text fields.
	owner map[string]any
}

// responsesTextPartTypes are the content part types whose "text" is prompt
// text; refusal parts carry theirs in "refusal".
var responsesTextPartTypes = map[string]bool{
	"input_text": true, "output_text": true, "text": true, "summary_text": true, "reasoning_text": true,
}

// responsesSlots decodes body and lists its prompt texts in body order. obj
// is the decoded body the slots write into.
func responsesSlots(body []byte) (map[string]any, []responsesSlot, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, nil, errors.New("not a JSON object")
	}
	var slots []responsesSlot

	switch v := obj["instructions"].(type) {
	case nil:
	case string:
		slots = append(slots, responsesSlot{role: llms.ChatMessageTypeSystem, text: v, set: func(s string) { obj["instructions"] = s }})
	default:
		return nil, nil, errors.New("instructions must be a string")
	}

	if p, ok := obj["prompt"]; ok && p != nil {
		prompt, ok := p.(map[string]any)
		if !ok {
			return nil, nil, errors.New("prompt must be an object")
		}
		if vars, ok := prompt["variables"]; ok && vars != nil {
			m, ok := vars.(map[string]any)
			if !ok {
				return nil, nil, errors.New("prompt.variables must be an object")
			}
			names := make([]string, 0, len(m))
			for name := range m {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				switch v := m[name].(type) {
				case string:
					slots = append(slots, responsesSlot{role: llms.ChatMessageTypeHuman, text: v, set: func(s string) { m[name] = s }})
				case map[string]any:
					s, err := responsesPartSlots([]any{v}, llms.ChatMessageTypeHuman)
					if err != nil {
						return nil, nil, fmt.Errorf("prompt.variables.%s: %w", name, err)
					}
					slots = append(slots, s...)
				default:
					return nil, nil, fmt.Errorf("prompt.variables.%s has an unsupported type", name)
				}
			}
		}
	}

	switch in := obj["input"].(type) {
	case nil:
	case string:
		slots = append(slots, responsesSlot{role: llms.ChatMessageTypeHuman, text: in, set: func(s string) { obj["input"] = s }})
	case []any:
		// Tool outputs carry only the call's id; the call names the tool.
		names := map[string]string{}
		for _, el := range in {
			if item, ok := el.(map[string]any); ok {
				id, _ := item["call_id"].(string)
				name, _ := item["name"].(string)
				if id != "" && name != "" {
					names[id] = name
				}
			}
		}
		for i, el := range in {
			item, ok := el.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("input[%d] must be an object", i)
			}
			s, err := responsesItemSlots(item, names)
			if err != nil {
				return nil, nil, fmt.Errorf("input[%d]: %w", i, err)
			}
			slots = append(slots, s...)
		}
	default:
		return nil, nil, errors.New("input must be a string or an array")
	}
	return obj, slots, nil
}

// responsesItemSlots lists the prompt texts of one input item. names maps
// the request's tool call ids to the names of the tools called.
func responsesItemSlots(item map[string]any, names map[string]string) ([]responsesSlot, error) {
	typ, _ := item["type"].(string)
	if _, ok := item["type"]; ok && typ == "" {
		return nil, errors.New("type must be a string")
	}
	role, hasRole := item["role"].(string)
	switch {
	case typ == "message" || (typ == "" && hasRole):
		r, err := responsesRole(role)
		if err != nil {
			return nil, err
		}
		switch c := item["content"].(type) {
		case string:
			return []responsesSlot{{role: r, text: c, set: func(s string) { item["content"] = s }}}, nil
		case []any:
			return responsesContentSlot(item, c, r)
		default:
			return nil, errors.New("content must be a string or an array")
		}
	case strings.HasSuffix(typ, "_call_output"):
		// A tool result is untrusted input to the model.
		id, _ := item["call_id"].(string)
		tool := scripting.MessageTool{Name: names[id], CallID: id}
		var slots []responsesSlot
		switch out := item["output"].(type) {
		case nil:
			return nil, nil
		case string:
			slots = []responsesSlot{{role: llms.ChatMessageTypeTool, text: out, set: func(s string) { item["output"] = s }}}
		case []any:
			var err error
			if slots, err = responsesPartSlots(out, llms.ChatMessageTypeTool); err != nil {
				return nil, err
			}
		default:
			b, _ := json.Marshal(out)
			slots = []responsesSlot{{role: llms.ChatMessageTypeTool, text: string(b), set: func(s string) { item["output"] = s }}}
		}
		for i := range slots {
			slots[i].tool = tool
		}
		return slots, nil
	case typ == "function_call" || typ == "custom_tool_call":
		field := "arguments"
		if typ == "custom_tool_call" {
			field = "input"
		}
		switch a := item[field].(type) {
		case nil:
			return nil, nil
		case string:
			return []responsesSlot{{role: llms.ChatMessageTypeAI, text: a, set: func(s string) { item[field] = s }}}, nil
		default:
			return nil, fmt.Errorf("%s must be a string", field)
		}
	}
	fields, known := responsesItemTextFields[typ]
	if !known {
		// An item type this extractor does not know may carry text the
		// model reads (a type added to the API later): refuse it rather
		// than forward it unscreened.
		if typ == "" {
			return nil, errors.New("item has no type or role")
		}
		return nil, fmt.Errorf("unsupported item type %q", typ)
	}
	var slots []responsesSlot
	for _, f := range fields {
		s, err := responsesFieldSlots(item, strings.Split(f.path, "."), f.role)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
		if f.role == llms.ChatMessageTypeTool {
			for i := range s {
				s[i].tool = serverToolResult(typ, item, s[i].owner)
			}
		}
		slots = append(slots, s...)
	}
	return slots, nil
}

// serverToolResult names the tool behind a tool-result text of a built-in
// or MCP tool item: an MCP call's tool, a tool an MCP server lists (owner is
// the listed tool), or the built-in tool (file_search_call: file_search).
// The call id is the item's id.
func serverToolResult(typ string, item, owner map[string]any) scripting.MessageTool {
	id, _ := item["id"].(string)
	var name string
	switch typ {
	case "mcp_call":
		name, _ = item["name"].(string)
	case "mcp_list_tools":
		name, _ = owner["name"].(string)
	default:
		name = strings.TrimSuffix(typ, "_call")
	}
	return scripting.MessageTool{Name: name, CallID: id}
}

// responsesTextField is a text-bearing field of an input item. path is
// dot-separated; a segment ending in "[]" is an array to walk, its elements
// objects (or, on the last segment, strings).
type responsesTextField struct {
	path string
	role llms.ChatMessageType
}

// responsesItemTextFields lists, for every other input item type the
// extractor knows, the fields the model reads: what the model produced as
// assistant text, tool and server results as tool text. Types with no entry
// fields (item_reference, image_generation_call) carry no prompt text. A type
// missing from the map is refused.
var responsesItemTextFields = map[string][]responsesTextField{
	"item_reference":        nil,
	"image_generation_call": nil,
	"reasoning": {
		{"summary[].text", llms.ChatMessageTypeAI},
		{"content[].text", llms.ChatMessageTypeAI},
	},
	"mcp_call": {
		{"arguments", llms.ChatMessageTypeAI},
		{"output", llms.ChatMessageTypeTool},
		{"error", llms.ChatMessageTypeTool},
	},
	"mcp_list_tools": {
		{"tools[].description", llms.ChatMessageTypeTool},
		{"error", llms.ChatMessageTypeTool},
	},
	"mcp_approval_request":  {{"arguments", llms.ChatMessageTypeAI}},
	"mcp_approval_response": {{"reason", llms.ChatMessageTypeHuman}},
	"code_interpreter_call": {
		{"code", llms.ChatMessageTypeAI},
		{"outputs[].logs", llms.ChatMessageTypeTool},
	},
	"web_search_call": {
		{"action.query", llms.ChatMessageTypeAI},
		{"action.queries[]", llms.ChatMessageTypeAI},
	},
	"file_search_call": {
		{"queries[]", llms.ChatMessageTypeAI},
		{"results[].text", llms.ChatMessageTypeTool},
	},
	"computer_call":    {{"action.text", llms.ChatMessageTypeAI}},
	"local_shell_call": {{"action.command[]", llms.ChatMessageTypeAI}},
}

// responsesFieldSlots lists the strings at path under obj, one slot each.
// A missing or null field has none; any other non-string where text is
// expected is an error (fail closed).
func responsesFieldSlots(obj map[string]any, path []string, role llms.ChatMessageType) ([]responsesSlot, error) {
	key, isArray := strings.CutSuffix(path[0], "[]")
	v, ok := obj[key]
	if !ok || v == nil {
		return nil, nil
	}
	last := len(path) == 1
	if !isArray {
		if last {
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			return []responsesSlot{{role: role, text: str, set: func(s string) { obj[key] = s }, owner: obj}}, nil
		}
		child, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return responsesFieldSlots(child, path[1:], role)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	var slots []responsesSlot
	for i, el := range arr {
		if last {
			str, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string", key, i)
			}
			slots = append(slots, responsesSlot{role: role, text: str, set: func(s string) { arr[i] = s }, owner: obj})
			continue
		}
		child, ok := el.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", key, i)
		}
		s, err := responsesFieldSlots(child, path[1:], role)
		if err != nil {
			return nil, err
		}
		slots = append(slots, s...)
	}
	return slots, nil
}

// responsesContentSlot is a message's text parts as one slot. A rewrite
// replaces them with one text part (input_text, or output_text for the
// assistant) ahead of the message's other parts (images, files).
func responsesContentSlot(item map[string]any, parts []any, role llms.ChatMessageType) ([]responsesSlot, error) {
	var texts []string
	var others []any
	for i, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("content[%d] must be an object", i)
		}
		t, isText, err := responsesPartText(part)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
		if isText {
			texts = append(texts, t)
		} else {
			others = append(others, part)
		}
	}
	if len(texts) == 0 {
		return nil, nil
	}
	partType := "input_text"
	if role == llms.ChatMessageTypeAI {
		partType = "output_text"
	}
	return []responsesSlot{{role: role, text: strings.Join(texts, "\n"), set: func(s string) {
		content := append([]any{map[string]any{"type": partType, "text": s}}, others...)
		item["content"] = content
	}}}, nil
}

// responsesPartSlots lists text parts (of a tool output or a prompt
// variable) as one slot each.
func responsesPartSlots(parts []any, role llms.ChatMessageType) ([]responsesSlot, error) {
	var slots []responsesSlot
	for i, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("part %d must be an object", i)
		}
		t, isText, err := responsesPartText(part)
		if err != nil {
			return nil, fmt.Errorf("part %d: %w", i, err)
		}
		if !isText {
			continue
		}
		field := "text"
		if part["type"] == "refusal" {
			field = "refusal"
		}
		slots = append(slots, responsesSlot{role: role, text: t, set: func(s string) { part[field] = s }})
	}
	return slots, nil
}

// responsesPartText is a content part's text, if it is a text part.
func responsesPartText(part map[string]any) (string, bool, error) {
	typ, _ := part["type"].(string)
	field := "text"
	switch {
	case typ == "refusal":
		field = "refusal"
	case responsesTextPartTypes[typ]:
	default:
		// Images, files, audio: not text. An unknown type with a text is
		// still read.
		if _, ok := part["text"]; !ok {
			return "", false, nil
		}
	}
	t, ok := part[field].(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", field)
	}
	return t, true, nil
}

func responsesRole(role string) (llms.ChatMessageType, error) {
	switch role {
	case "user", "":
		return llms.ChatMessageTypeHuman, nil
	case "assistant":
		return llms.ChatMessageTypeAI, nil
	case "system", "developer":
		return llms.ChatMessageTypeSystem, nil
	}
	return "", fmt.Errorf("unsupported role %q", role)
}

// OpenAIResponsesMessageExtractor reads OpenAI Responses API requests.
type OpenAIResponsesMessageExtractor struct{}

func (e *OpenAIResponsesMessageExtractor) VendorName() string {
	return requestFilterFormatOpenAIResponses
}

// failClosed marks the extractor's errors as refusing the request.
func (e *OpenAIResponsesMessageExtractor) failClosed() bool { return true }

func (e *OpenAIResponsesMessageExtractor) ExtractMessages(_ *http.Request, body []byte) ([]llms.MessageContent, error) {
	_, slots, err := responsesSlots(body)
	if err != nil {
		return nil, fmt.Errorf("invalid Responses API request: %w", err)
	}
	out := make([]llms.MessageContent, len(slots))
	for i, s := range slots {
		out[i] = llms.MessageContent{Role: s.role, Parts: []llms.ContentPart{llms.TextPart(s.text)}}
	}
	return out, nil
}

// messageTools names the tool behind each message ExtractMessages returns;
// nil when the body cannot be read.
func (e *OpenAIResponsesMessageExtractor) messageTools(body []byte) []scripting.MessageTool {
	_, slots, err := responsesSlots(body)
	if err != nil {
		return nil
	}
	out := make([]scripting.MessageTool, len(slots))
	for i, s := range slots {
		out[i] = s.tool
	}
	return out
}

// OpenAIResponsesMessageReconstructor writes filter edits back into a
// Responses API request. The filter output must keep the extracted messages
// in order (guardrails and redacting filters do); each changed text goes
// back into its field. Output that adds or drops messages cannot be mapped
// onto the request's fields and fails the request.
type OpenAIResponsesMessageReconstructor struct{}

func (r *OpenAIResponsesMessageReconstructor) VendorName() string {
	return requestFilterFormatOpenAIResponses
}

func (r *OpenAIResponsesMessageReconstructor) Reconstruct(messages []map[string]interface{}, originalBody []byte) ([]byte, error) {
	obj, slots, err := responsesSlots(originalBody)
	if err != nil {
		return nil, fmt.Errorf("invalid Responses API request: %w", err)
	}
	if len(messages) != len(slots) {
		return nil, fmt.Errorf("a filter returned %d messages for a Responses API request with %d; only edits in place can be written back", len(messages), len(slots))
	}
	for i, m := range messages {
		content, ok := m["content"].(string)
		if !ok {
			return nil, fmt.Errorf("message %d has no text content", i)
		}
		if content != slots[i].text {
			slots[i].set(content)
		}
	}
	return json.Marshal(obj)
}
