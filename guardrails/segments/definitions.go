package segments

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// Tool definitions (Options.ToolDefinitions): each tool a request offers the
// model is a unit of Role system and Kind tool_definition, named after the
// tool, whose text is the definition's raw JSON (name, description and
// parameter schema as the model is shown them). A definition is never
// rewritten, only removed from its array (RemovalEdits).

// ErrUnremovable is a removal that cannot be written into the body: it
// would leave a tools array empty, or the request forces a tool it removes
// (tool_choice). The caller blocks the request instead.
var ErrUnremovable = errors.New("segments: the tool definitions cannot be removed in place")

// definition opens a unit for the tool definition def, element elem of the
// array at arrayPath, named name.
func (x *extraction) definition(def *node, arrayPath string, elem int, name string) {
	if !x.opts.ToolDefinitions || !def.is(nodeObject) {
		return
	}
	path := pointerIndex(arrayPath, elem)
	u := x.open(path, wholeNone, guardrails.Segment{Role: guardrails.RoleSystem, Kind: guardrails.KindToolDefinition, ToolName: name})
	u.Message = -1
	u.removeArray, u.removeElem = arrayPath, elem
	x.detect(u, string(x.body[def.start:def.end]))
}

// definitions reads the tools of a request root in format.
func (x *extraction) definitions(format string, root *node) error {
	if !x.opts.ToolDefinitions {
		return nil
	}
	switch format {
	case FormatOpenAI, FormatOllama:
		if err := x.strict(root, "", "tools", "functions"); err != nil {
			return err
		}
		for i, t := range root.get("tools").elems() {
			fn := t.get("function")
			name := x.str(fn.get("name"))
			if name == "" {
				name = x.str(t.get("name")) // custom and other tool types
			}
			x.definition(t, "/tools", i, name)
		}
		for i, fn := range root.get("functions").elems() {
			x.definition(fn, "/functions", i, x.str(fn.get("name")))
		}
	case FormatAnthropic:
		if err := x.strict(root, "", "tools"); err != nil {
			return err
		}
		for i, t := range root.get("tools").elems() {
			x.definition(t, "/tools", i, x.str(t.get("name")))
		}
	case FormatGoogleAI, FormatVertex:
		if err := x.strict(root, "", "tools"); err != nil {
			return err
		}
		for i, t := range root.get("tools").elems() {
			decls, key, err := x.alias(t, pointerIndex("/tools", i), "functionDeclarations", "function_declarations")
			if err != nil {
				return err
			}
			arr := pointer(pointerIndex("/tools", i), key)
			for j, d := range decls.elems() {
				x.definition(d, arr, j, x.str(d.get("name")))
			}
		}
	case FormatBedrock:
		if root.get("anthropic_version") != nil {
			return x.definitions(FormatAnthropic, root)
		}
		if err := x.strict(root, "", "toolConfig"); err != nil {
			return err
		}
		for i, t := range root.get("toolConfig").get("tools").elems() {
			if spec := t.get("toolSpec"); spec.is(nodeObject) {
				x.definition(t, "/toolConfig/tools", i, x.str(spec.get("name")))
			}
		}
	}
	return nil
}

// ResponsesToolDefinitions reads the function and custom tools of an OpenAI
// Responses request (tools[]), as units like the other formats'. Built-in
// and MCP tool entries name no definition the model reads and are skipped.
func ResponsesToolDefinitions(body []byte) ([]Unit, error) {
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	if root.kind != nodeObject {
		return nil, fmt.Errorf("%w: not a JSON object", ErrUnrecognisedBody)
	}
	x := newExtraction(body)
	x.opts.ToolDefinitions = true
	for i, t := range root.get("tools").elems() {
		switch x.str(t.get("type")) {
		case "function", "custom":
			x.definition(t, "/tools", i, x.str(t.get("name")))
		}
	}
	x.finish()
	return x.result(), nil
}

// RemovalEdits are the edits that drop the definition units from their
// arrays in body (format names the request format, for tool_choice). It
// returns ErrUnremovable when a removal would leave an array empty or the
// request forces a removed tool.
func RemovalEdits(format string, body []byte, units []Unit) ([]Edit, error) {
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	drop := map[string]map[int]bool{}
	removed := map[string]bool{}
	for _, u := range units {
		if u.removeArray == "" {
			return nil, fmt.Errorf("%w: unit %d is not a tool definition", ErrUnremovable, u.Index)
		}
		if drop[u.removeArray] == nil {
			drop[u.removeArray] = map[int]bool{}
		}
		drop[u.removeArray][u.removeElem] = true
		removed[u.ToolName] = true
	}
	if forced := forcedTool(format, root, body); forced != "" && removed[forced] {
		return nil, fmt.Errorf("%w: tool_choice forces a removed tool", ErrUnremovable)
	}
	var edits []Edit
	for path, elems := range drop {
		arr := root.resolve(path)
		if !arr.is(nodeArray) {
			return nil, fmt.Errorf("%w: %s is not an array", ErrUnremovable, maskPath(path))
		}
		var kept []string
		for i, v := range arr.vals {
			if !elems[i] {
				kept = append(kept, string(body[v.start:v.end]))
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("%w: it would leave %s empty", ErrUnremovable, maskPath(path))
		}
		edits = append(edits, Edit{Path: path, Value: []byte("[" + strings.Join(kept, ",") + "]")})
	}
	return edits, nil
}

// forcedTool is the tool a request forces the model to call, if any:
// OpenAI tool_choice {"type": "function", "function": {"name"}} (Responses:
// {"type": "function", "name"}), Anthropic {"type": "tool", "name"}, Bedrock
// toolConfig.toolChoice.tool.name. Google's allowedFunctionNames restricts
// rather than forces, and is checked by the vendor.
func forcedTool(format string, root *node, body []byte) string {
	x := &extraction{body: body}
	tc := root.get("tool_choice")
	switch {
	case tc.is(nodeObject) && x.str(tc.get("function").get("name")) != "":
		return x.str(tc.get("function").get("name"))
	case tc.is(nodeObject) && x.str(tc.get("name")) != "":
		return x.str(tc.get("name"))
	}
	if format == FormatBedrock {
		return x.str(root.get("toolConfig").get("toolChoice").get("tool").get("name"))
	}
	return ""
}
