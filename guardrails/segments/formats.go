package segments

import (
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// The fields each format reads, per object. strict refuses a case variant of
// any of them.
var (
	openAIRootFields     = []string{"messages"}
	openAIMessageFields  = []string{"role", "content", "name", "tool_calls", "tool_call_id", "tool_name", "refusal", "function_call"}
	openAIPartFields     = []string{"type", "text", "refusal"}
	openAIToolCallFields = []string{"id", "type", "function"}
	openAIFunctionFields = []string{"name", "arguments"}
	ollamaGenerateFields = []string{"messages", "prompt", "suffix", "system", "template"}

	anthropicRootFields    = []string{"messages", "system"}
	anthropicMessageFields = []string{"role", "content"}
	anthropicBlockFields   = []string{"type", "text", "source", "content", "tool_use_id", "id", "name", "title", "context", "url"}
	anthropicSourceFields  = []string{"type", "data", "content", "media_type"}

	googleRootFields     = []string{"contents", "systemInstruction", "system_instruction"}
	googleContentFields  = []string{"role", "parts"}
	googleResponseFields = []string{"name", "id", "response"}
	googleCallFields     = []string{"name", "id", "args"}
	googleCodeFields     = []string{"outcome", "output"}
)

// OpenAI chat completions: messages[].content is a string or an array of
// parts; a role "tool" message answers tool_calls[].id through tool_call_id,
// and a legacy role "function" message names its function in name. Each
// message is one unit.
func extractOpenAI(x *extraction, root *node) error {
	if err := x.strict(root, "", openAIRootFields...); err != nil {
		return err
	}
	msgs, err := x.required(root, "messages")
	if err != nil {
		return err
	}
	for i, m := range msgs {
		path := pointerIndex("/messages", i)
		if err := x.strict(m, path, openAIMessageFields...); err != nil {
			return err
		}
		for j, call := range m.get("tool_calls").elems() {
			callPath := pointerIndex(pointer(path, "tool_calls"), j)
			if err := x.strict(call, callPath, openAIToolCallFields...); err != nil {
				return err
			}
			if err := x.strict(call.get("function"), pointer(callPath, "function"), openAIFunctionFields...); err != nil {
				return err
			}
			x.noteCall(call.get("id"), call.get("function").get("name"))
		}
	}
	for i, m := range msgs {
		x.enter(i)
		path := pointerIndex("/messages", i)
		role := messageRole(x.str(m.get("role")))
		meta := guardrails.Segment{Role: role}
		if role == guardrails.RoleTool {
			// OpenAI tool messages carry only the call id; Ollama names the
			// tool in tool_name, a legacy function message in name.
			name := x.str(m.get("tool_name"))
			if name == "" {
				name = x.str(m.get("name"))
			}
			meta = x.toolResult(x.str(m.get("tool_call_id")), name)
		}
		u := x.open(pointer(path, "content"), wholeString, meta)
		u.adjacent = true
		if err := x.openAIContent(u, m.get("content"), pointer(path, "content")); err != nil {
			return err
		}
		x.leaf(u, m.get("refusal"), pointer(path, "refusal"))
		// The calls the model made (OpenAI arguments are a JSON string,
		// Ollama's an object), and a legacy function_call.
		for j, call := range m.get("tool_calls").elems() {
			fnPath := pointer(pointerIndex(pointer(path, "tool_calls"), j), "function")
			fn := call.get("function")
			x.toolCall(fn.get("arguments"), pointer(fnPath, "arguments"), call.get("id"), fn.get("name"))
		}
		if fc := m.get("function_call"); fc.is(nodeObject) {
			x.toolCall(fc.get("arguments"), pointer(pointer(path, "function_call"), "arguments"), nil, fc.get("name"))
		}
		x.leave(role)
	}
	return nil
}

// openAIContent adds a message's content to u: a string, or an array of
// parts. Text and refusal parts are leaves; image, audio and file parts are
// not text. A part of any other type joins u with all its strings.
func (x *extraction) openAIContent(u *Unit, n *node, path string) error {
	if x.isNull(n) {
		return nil
	}
	switch n.kind {
	case nodeString:
		x.leaf(u, n, path)
		return nil
	case nodeArray:
		parts, err := x.objects(n, path)
		if err != nil {
			return err
		}
		for k, part := range parts {
			pp := pointerIndex(path, k)
			if err := x.strict(part, pp, openAIPartFields...); err != nil {
				return err
			}
			switch x.str(part.get("type")) {
			case "text":
				x.leaf(u, part.get("text"), pointer(pp, "text"))
			case "refusal":
				x.leaf(u, part.get("refusal"), pointer(pp, "refusal"))
			case "image_url", "input_audio", "file":
			default:
				x.walk(u, part, pp, true, opaque)
			}
		}
		return nil
	}
	return shapeError(path, "a string or an array")
}

// Ollama /api/chat is the OpenAI message shape (tool messages name the tool
// in tool_name); /api/generate carries a prompt, a suffix, a system string and
// a template instead of messages.
func extractOllama(x *extraction, root *node) error {
	if root.get("messages") != nil || root.get("prompt") == nil {
		return extractOpenAI(x, root) // a missing messages array is reported there
	}
	if err := x.strict(root, "", ollamaGenerateFields...); err != nil {
		return err
	}
	system := x.open("", wholeLeaves, guardrails.Segment{Role: guardrails.RoleSystem})
	x.leaf(system, root.get("system"), "/system")
	x.leaf(system, root.get("template"), "/template")
	x.enter(0)
	user := x.open("", wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
	x.leaf(user, root.get("prompt"), "/prompt")
	x.leaf(user, root.get("suffix"), "/suffix")
	return nil
}

// Anthropic messages: system is a string or text blocks; messages[].content is
// a string or an array of blocks. Each message's text is one unit and each
// tool result another: tool_result blocks in user messages (answering a
// tool_use block's id through tool_use_id), and the server tools' results
// the client resends in assistant messages (mcp_tool_result,
// web_fetch_tool_result, web_search_tool_result, the code execution results,
// and any other "*_tool_result").
func extractAnthropic(x *extraction, root *node) error {
	if err := x.strict(root, "", anthropicRootFields...); err != nil {
		return err
	}
	msgs, err := x.required(root, "messages")
	if err != nil {
		return err
	}
	for i, m := range msgs {
		path := pointerIndex("/messages", i)
		if err := x.strict(m, path, anthropicMessageFields...); err != nil {
			return err
		}
		for _, b := range m.get("content").elems() {
			switch x.str(b.get("type")) {
			case "tool_use", "server_tool_use", "mcp_tool_use":
				x.noteCall(b.get("id"), b.get("name"))
			}
		}
	}
	system := x.open("/system", wholeString, guardrails.Segment{Role: guardrails.RoleSystem})
	system.adjacent = true
	if err := x.anthropicContent(system, root.get("system"), "/system"); err != nil {
		return err
	}
	for i, m := range msgs {
		x.enter(i)
		path := pointerIndex("/messages", i)
		role := guardrails.RoleUser
		if x.str(m.get("role")) == "assistant" {
			role = guardrails.RoleAssistant
		}
		u := x.open(pointer(path, "content"), wholeLeaves, guardrails.Segment{Role: role})
		u.adjacent = true
		if err := x.anthropicContent(u, m.get("content"), pointer(path, "content")); err != nil {
			return err
		}
		x.leave(role)
	}
	return nil
}

// anthropicContent adds a string or an array of content blocks to u.
func (x *extraction) anthropicContent(u *Unit, n *node, path string) error {
	if x.isNull(n) {
		return nil
	}
	switch n.kind {
	case nodeString:
		x.leaf(u, n, path)
		return nil
	case nodeArray:
		blocks, err := x.objects(n, path)
		if err != nil {
			return err
		}
		for k, b := range blocks {
			if err := x.anthropicBlock(u, b, pointerIndex(path, k)); err != nil {
				return err
			}
		}
		return nil
	}
	return shapeError(path, "a string or an array")
}

// anthropicBlock adds one content block to u, or opens a unit for a tool
// result. Images, thinking, tool calls and container uploads carry no text a
// guardrail reads.
func (x *extraction) anthropicBlock(u *Unit, b *node, path string) error {
	if err := x.strict(b, path, anthropicBlockFields...); err != nil {
		return err
	}
	contentPath := pointer(path, "content")
	switch typ := x.str(b.get("type")); typ {
	case "text":
		x.leaf(u, b.get("text"), pointer(path, "text"))
	case "tool_use", "server_tool_use", "mcp_tool_use":
		if x.opts.ToolCalls {
			if err := x.strict(b, path, "input"); err != nil {
				return err
			}
		}
		x.toolCall(b.get("input"), pointer(path, "input"), b.get("id"), b.get("name"))
	case "image", "thinking", "redacted_thinking", "container_upload":
	case "document":
		return x.anthropicDocument(u, b, path)
	case "search_result":
		x.leaf(u, b.get("title"), pointer(path, "title"))
		return x.anthropicContent(u, b.get("content"), contentPath)
	case "tool_result", "mcp_tool_result":
		result := x.open(contentPath, wholeString, x.toolResult(x.str(b.get("tool_use_id")), ""))
		result.adjacent = true
		if typ == "mcp_tool_result" {
			result.ServerTool, result.ServerToolType = true, typ
		}
		return x.anthropicContent(result, b.get("content"), contentPath)
	case "web_fetch_tool_result":
		result := x.open(contentPath, wholeLeaves, x.toolResult(x.str(b.get("tool_use_id")), ""))
		result.ServerTool, result.ServerToolType = true, typ
		c := b.get("content")
		if x.str(c.get("type")) == "web_fetch_result" && c.get("content").is(nodeObject) {
			x.leaf(result, c.get("url"), pointer(contentPath, "url"))
			return x.anthropicDocument(result, c.get("content"), pointer(contentPath, "content"))
		}
		x.walk(result, c, contentPath, false, opaque)
	default:
		if strings.HasSuffix(typ, "_tool_result") {
			// web_search_tool_result (titles and URLs; the page content is
			// encrypted), the code execution results (stdout, stderr, file
			// text) and result types added later.
			result := x.open(contentPath, wholeLeaves, x.toolResult(x.str(b.get("tool_use_id")), ""))
			result.ServerTool, result.ServerToolType = true, typ
			x.walk(result, b.get("content"), contentPath, false, opaque)
			return nil
		}
		x.walk(u, b, path, false, opaque)
	}
	return nil
}

// anthropicDocument adds a document block: its title and context, and its
// text when the source is plain text or a content array. Base64, URL and
// file sources are not text.
func (x *extraction) anthropicDocument(u *Unit, b *node, path string) error {
	if err := x.strict(b, path, anthropicBlockFields...); err != nil {
		return err
	}
	x.leaf(u, b.get("title"), pointer(path, "title"))
	x.leaf(u, b.get("context"), pointer(path, "context"))
	src := b.get("source")
	srcPath := pointer(path, "source")
	if err := x.strict(src, srcPath, anthropicSourceFields...); err != nil {
		return err
	}
	switch x.str(src.get("type")) {
	case "text":
		x.leaf(u, src.get("data"), pointer(srcPath, "data"))
	case "content":
		return x.anthropicContent(u, src.get("content"), pointer(srcPath, "content"))
	case "base64":
		if strings.HasPrefix(x.str(src.get("media_type")), "text/") {
			x.decodedText(u, src.get("data"))
		}
	case "url", "file":
	default:
		x.walk(u, src, srcPath, false, opaque)
	}
	return nil
}

// googlePartData are the members of a Google part that carry no text a
// guardrail reads (model-generated calls and code, media, metadata), in both
// proto JSON spellings.
var googlePartData = map[string]bool{
	"functionCall": true, "function_call": true,
	"executableCode": true, "executable_code": true,
	"inlineData": true, "inline_data": true,
	"fileData": true, "file_data": true,
	"thought": true, "thoughtSignature": true, "thought_signature": true,
	"videoMetadata": true, "video_metadata": true,
	"mediaResolution": true, "media_resolution": true,
	"partMetadata": true, "part_metadata": true,
}

// Google AI / Vertex generateContent: systemInstruction and contents[] each
// hold parts[]. A content's text parts are one unit; each functionResponse
// and codeExecutionResult is a tool result unit of its own. Proto JSON
// accepts a field's name and its lowerCamelCase form, so both spellings are
// read, and a part with both is refused.
func extractGoogle(x *extraction, root *node) error {
	if err := x.strict(root, "", googleRootFields...); err != nil {
		return err
	}
	contents, err := x.required(root, "contents")
	if err != nil {
		return err
	}
	sys, key, err := x.alias(root, "", "systemInstruction", "system_instruction")
	if err != nil {
		return err
	}
	if !x.isNull(sys) {
		if sys.kind != nodeObject {
			return shapeError(pointer("", key), "an object")
		}
		if err := x.googleContent(sys, pointer("", key), guardrails.RoleSystem); err != nil {
			return err
		}
	}
	for i, c := range contents {
		x.enter(i)
		role := guardrails.RoleUser
		if x.str(c.get("role")) == "model" {
			role = guardrails.RoleAssistant
		}
		if err := x.googleContent(c, pointerIndex("/contents", i), role); err != nil {
			return err
		}
		x.leave(role)
	}
	return nil
}

func (x *extraction) googleContent(content *node, path, role string) error {
	if err := x.strict(content, path, googleContentFields...); err != nil {
		return err
	}
	partsPath := pointer(path, "parts")
	parts, err := x.objects(content.get("parts"), partsPath)
	if err != nil {
		return err
	}
	u := x.open(partsPath, wholeLeaves, guardrails.Segment{Role: role})
	u.adjacent = true
	for k, part := range parts {
		if err := x.googlePart(u, part, pointerIndex(partsPath, k)); err != nil {
			return err
		}
	}
	return nil
}

func (x *extraction) googlePart(u *Unit, part *node, path string) error {
	if err := x.strict(part, path, "text"); err != nil {
		return err
	}
	fr, frKey, err := x.alias(part, path, "functionResponse", "function_response")
	if err != nil {
		return err
	}
	cer, cerKey, err := x.alias(part, path, "codeExecutionResult", "code_execution_result")
	if err != nil {
		return err
	}
	fc, fcKey, err := x.alias(part, path, "functionCall", "function_call")
	if err != nil {
		return err
	}
	for i, key := range part.keys {
		val, memberPath := part.vals[i], pointer(path, key)
		switch {
		case key == "text":
			x.leaf(u, val, memberPath)
		case key == fcKey && val == fc && x.opts.ToolCalls:
			if err := x.strict(fc, memberPath, googleCallFields...); err != nil {
				return err
			}
			x.toolCall(fc.get("args"), pointer(memberPath, "args"), fc.get("id"), fc.get("name"))
		case key == frKey && val == fr:
			if err := x.strict(fr, memberPath, googleResponseFields...); err != nil {
				return err
			}
			// The model reads the whole response object, keys included.
			responsePath := pointer(memberPath, "response")
			result := x.open(responsePath, wholeObject, x.toolResult(x.str(fr.get("id")), x.str(fr.get("name"))))
			x.walk(result, fr.get("response"), responsePath, true, nil)
		case key == cerKey && val == cer:
			if err := x.strict(cer, memberPath, googleCodeFields...); err != nil {
				return err
			}
			result := x.open(memberPath, wholeLeaves, x.toolResult("", "code_execution"))
			result.ServerTool, result.ServerToolType = true, "codeExecutionResult"
			x.leaf(result, cer.get("output"), pointer(memberPath, "output"))
		case googlePartData[key]:
		default:
			x.walk(u, val, memberPath, true, opaque)
		}
	}
	return nil
}
