package segments

// Helpers for plugins that rewrite request bodies (the Prompt Compressor):
// a check that tool calls and results still pair up, where the prompt-cache
// breakpoints are, and whether the client already lets the vendor manage the
// context.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ErrToolPairing: a tool call has no result or more than one, a result
// answers no call, or two calls share an id.
var ErrToolPairing = errors.New("segments: tool calls and results do not pair")

// maxPairingIDs bounds the ids an ErrToolPairing message names.
const maxPairingIDs = 5

// CheckToolPairing reports ErrToolPairing unless every tool call in body has
// exactly one result and every result answers a call:
//
//   - OpenAI (and Ollama): an assistant message's tool_calls[].id and a tool
//     message's tool_call_id. Ollama's id-less calls and results are not
//     checked.
//   - Anthropic (and Bedrock InvokeModel with an Anthropic body): tool_use,
//     server_tool_use and mcp_tool_use blocks' id and the tool_use_id of
//     tool_result and every other *_tool_result block.
//   - Google AI / Vertex: functionCall.id and functionResponse.id. Where ids
//     are absent, the calls of a model content pair by position with the
//     id-less responses that follow it.
//   - Bedrock Converse: toolUse.toolUseId and toolResult.toolUseId.
//
// Other formats are ErrUnsupportedFormat; a body that is not JSON, or not an
// object, is ErrUnrecognisedBody, and a repeated key ErrDuplicateKey.
func CheckToolPairing(format string, body []byte) error {
	root, err := parseTree(body)
	if err != nil {
		return err
	}
	if root.kind != nodeObject {
		return fmt.Errorf("%w: not a JSON object", ErrUnrecognisedBody)
	}
	x := newExtraction(body)
	p := &pairing{calls: map[string]int{}, results: map[string]int{}}
	switch f := strings.ToLower(format); {
	case f == FormatOpenAI || f == FormatOllama:
		x.openAIPairs(root, p)
	case f == FormatAnthropic || f == FormatBedrock && root.get("anthropic_version") != nil:
		x.anthropicPairs(root, p)
	case f == FormatGoogleAI || f == FormatVertex:
		x.googlePairs(root, p)
	case f == FormatBedrock:
		x.bedrockPairs(root, p)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	return p.check()
}

// pairing counts the calls and results seen per id.
type pairing struct {
	calls, results map[string]int
	// unpaired are problems found without ids (Google position pairing).
	unpaired []string
}

func (p *pairing) call(id string)   { p.calls[id]++ }
func (p *pairing) result(id string) { p.results[id]++ }

func (p *pairing) check() error {
	problems := append([]string(nil), p.unpaired...)
	ids := make([]string, 0, len(p.calls)+len(p.results))
	for id := range p.calls {
		ids = append(ids, id)
	}
	for id := range p.results {
		if _, ok := p.calls[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		calls, results := p.calls[id], p.results[id]
		switch {
		case calls > 1:
			problems = append(problems, fmt.Sprintf("%d calls with id %q", calls, id))
		case calls == 0:
			problems = append(problems, fmt.Sprintf("result for no call %q", id))
		case results == 0:
			problems = append(problems, fmt.Sprintf("no result for call %q", id))
		case results > 1:
			problems = append(problems, fmt.Sprintf("%d results for call %q", results, id))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	more := ""
	if len(problems) > maxPairingIDs {
		more = fmt.Sprintf(" and %d more", len(problems)-maxPairingIDs)
		problems = problems[:maxPairingIDs]
	}
	return fmt.Errorf("%w: %s%s", ErrToolPairing, strings.Join(problems, "; "), more)
}

func (x *extraction) openAIPairs(root *node, p *pairing) {
	for _, m := range root.get("messages").elems() {
		for _, call := range m.get("tool_calls").elems() {
			if id := x.str(call.get("id")); id != "" {
				p.call(id)
			}
		}
		if x.str(m.get("role")) == "tool" {
			if id := x.str(m.get("tool_call_id")); id != "" {
				p.result(id)
			} else if m.get("tool_name") == nil {
				// OpenAI requires the id; only Ollama names the tool instead.
				p.unpaired = append(p.unpaired, "tool message without tool_call_id")
			}
		}
	}
}

func (x *extraction) anthropicPairs(root *node, p *pairing) {
	for _, m := range root.get("messages").elems() {
		for _, b := range m.get("content").elems() {
			switch typ := x.str(b.get("type")); {
			case typ == "tool_use" || typ == "server_tool_use" || typ == "mcp_tool_use":
				p.call(x.str(b.get("id")))
			case typ == "tool_result" || strings.HasSuffix(typ, "_tool_result"):
				p.result(x.str(b.get("tool_use_id")))
			}
		}
	}
}

func (x *extraction) bedrockPairs(root *node, p *pairing) {
	for _, m := range root.get("messages").elems() {
		for _, b := range m.get("content").elems() {
			if use := b.get("toolUse"); use.is(nodeObject) {
				p.call(x.str(use.get("toolUseId")))
			}
			if res := b.get("toolResult"); res.is(nodeObject) {
				p.result(x.str(res.get("toolUseId")))
			}
		}
	}
}

// googlePairs pairs functionCall and functionResponse parts by id, and the
// id-less ones by position: the k-th id-less response after a model content
// answers that content's k-th id-less call.
func (x *extraction) googlePairs(root *node, p *pairing) {
	pending, answered := 0, 0 // id-less calls of the latest model content
	flush := func(at int) {
		if answered < pending {
			p.unpaired = append(p.unpaired, fmt.Sprintf("no response for %d id-less call(s) of content %d", pending-answered, at))
		}
	}
	lastModel := -1
	for i, c := range root.get("contents").elems() {
		model := x.str(c.get("role")) == "model"
		if model {
			flush(lastModel)
			pending, answered, lastModel = 0, 0, i
		}
		for _, part := range c.get("parts").elems() {
			if fc := googleMember(part, "functionCall", "function_call"); fc.is(nodeObject) {
				if id := x.str(fc.get("id")); id != "" {
					p.call(id)
				} else if model {
					pending++
				}
			}
			if fr := googleMember(part, "functionResponse", "function_response"); fr.is(nodeObject) {
				if id := x.str(fr.get("id")); id != "" {
					p.result(id)
				} else if answered < pending {
					answered++
				} else {
					p.unpaired = append(p.unpaired, fmt.Sprintf("id-less response in content %d answers no call", i))
				}
			}
		}
	}
	flush(lastModel)
}

// googleMember returns a part member in either proto JSON spelling.
func googleMember(n *node, camel, snake string) *node {
	if v := n.get(camel); v != nil {
		return v
	}
	return n.get(snake)
}

// Prompt sections, in the order the vendor builds the cached prefix.
const (
	SectionTools = iota
	SectionSystem
	SectionMessages
)

// Breakpoint is a prompt-cache breakpoint: the vendor caches the prompt up to
// and including Block of Message (in SectionMessages) or of the tools or
// system array.
type Breakpoint struct {
	// Path is the JSON Pointer to the object carrying cache_control, or to
	// the cachePoint block.
	Path    string
	Section int
	// Message is the message index in SectionMessages, -1 otherwise.
	Message int
	// Block is the block (or tool) index the prefix ends at; math.MaxInt
	// when the breakpoint covers the whole message (a message-level or
	// request-level cache_control).
	Block int
}

// CacheBreakpoints returns body's prompt-cache breakpoints in prompt order
// (tools, system, messages):
//
//   - Anthropic (and Bedrock InvokeModel with an Anthropic body):
//     cache_control on a tool, a system block, a message, a content block or
//     a block nested in a tool result. A request-level cache_control
//     (automatic caching) counts as a breakpoint at the end of the last
//     message.
//   - Bedrock Converse: cachePoint blocks in toolConfig.tools, system and
//     message content.
//
// OpenAI, Ollama and Google bodies carry no breakpoints (OpenAI caches
// prefixes automatically; Google's cached content lives outside the
// request), so they return none. Other formats are ErrUnsupportedFormat.
func CacheBreakpoints(format string, body []byte) ([]Breakpoint, error) {
	f := strings.ToLower(format)
	switch f {
	case FormatOpenAI, FormatOllama, FormatGoogleAI, FormatVertex:
		if err := CheckKeys(body); err != nil {
			return nil, err
		}
		return nil, nil
	case FormatAnthropic, FormatBedrock:
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	if root.kind != nodeObject {
		return nil, fmt.Errorf("%w: not a JSON object", ErrUnrecognisedBody)
	}
	var bps []Breakpoint
	if f == FormatAnthropic || root.get("anthropic_version") != nil {
		bps = anthropicBreakpoints(root)
	} else {
		bps = bedrockBreakpoints(root)
	}
	sort.SliceStable(bps, func(i, j int) bool { return bps[i].before(bps[j]) })
	return bps, nil
}

func (b Breakpoint) before(o Breakpoint) bool {
	if b.Section != o.Section {
		return b.Section < o.Section
	}
	if b.Message != o.Message {
		return b.Message < o.Message
	}
	return b.Block < o.Block
}

func anthropicBreakpoints(root *node) []Breakpoint {
	var bps []Breakpoint
	for i, t := range root.get("tools").elems() {
		if t.get("cache_control").is(nodeObject) {
			bps = append(bps, Breakpoint{Path: pointerIndex("/tools", i), Section: SectionTools, Message: -1, Block: i})
		}
	}
	for k, b := range root.get("system").elems() {
		if b.get("cache_control").is(nodeObject) {
			bps = append(bps, Breakpoint{Path: pointerIndex("/system", k), Section: SectionSystem, Message: -1, Block: k})
		}
	}
	msgs := root.get("messages").elems()
	for i, m := range msgs {
		path := pointerIndex("/messages", i)
		if m.get("cache_control").is(nodeObject) {
			bps = append(bps, Breakpoint{Path: path, Section: SectionMessages, Message: i, Block: math.MaxInt})
		}
		for k, b := range m.get("content").elems() {
			bp := pointerIndex(pointer(path, "content"), k)
			if b.get("cache_control").is(nodeObject) {
				bps = append(bps, Breakpoint{Path: bp, Section: SectionMessages, Message: i, Block: k})
				continue
			}
			// A block inside a tool result caches the whole result.
			for j, inner := range b.get("content").elems() {
				if inner.get("cache_control").is(nodeObject) {
					bps = append(bps, Breakpoint{Path: pointerIndex(pointer(bp, "content"), j), Section: SectionMessages, Message: i, Block: k})
					break
				}
			}
		}
	}
	if root.get("cache_control").is(nodeObject) && len(msgs) > 0 {
		bps = append(bps, Breakpoint{Path: "/cache_control", Section: SectionMessages, Message: len(msgs) - 1, Block: math.MaxInt})
	}
	return bps
}

func bedrockBreakpoints(root *node) []Breakpoint {
	var bps []Breakpoint
	for i, t := range root.get("toolConfig").get("tools").elems() {
		if t.get("cachePoint") != nil {
			bps = append(bps, Breakpoint{Path: pointerIndex("/toolConfig/tools", i), Section: SectionTools, Message: -1, Block: i})
		}
	}
	for k, b := range root.get("system").elems() {
		if b.get("cachePoint") != nil {
			bps = append(bps, Breakpoint{Path: pointerIndex("/system", k), Section: SectionSystem, Message: -1, Block: k})
		}
	}
	for i, m := range root.get("messages").elems() {
		content := pointer(pointerIndex("/messages", i), "content")
		for k, b := range m.get("content").elems() {
			if b.get("cachePoint") != nil {
				bps = append(bps, Breakpoint{Path: pointerIndex(content, k), Section: SectionMessages, Message: i, Block: k})
			}
		}
	}
	return bps
}

// LastCacheBreakpoint returns the message the cached prefix of body ends in:
// a message index, -1 for the system prompt, -2 for the tools. ok is false
// when body has no breakpoint or cannot be read.
func LastCacheBreakpoint(format string, body []byte) (message int, ok bool) {
	bps, err := CacheBreakpoints(format, body)
	if err != nil || len(bps) == 0 {
		return 0, false
	}
	last := bps[len(bps)-1]
	switch last.Section {
	case SectionTools:
		return -2, true
	case SectionSystem:
		return -1, true
	}
	return last.Message, true
}

// Cached reports whether all of u lies in the prompt prefix bp caches, so
// rewriting it would change cached tokens. Units of a format without
// breakpoints never are.
func (u Unit) Cached(bp Breakpoint) bool {
	paths := make([]string, 0, len(u.Leaves)+1)
	for _, l := range u.Leaves {
		paths = append(paths, l.Path)
	}
	if len(paths) == 0 {
		paths = append(paths, u.Path)
	}
	for _, p := range paths {
		pos, ok := promptPosition(p)
		if !ok || bp.before(pos) {
			return false
		}
	}
	return true
}

// promptPosition places a JSON Pointer in prompt order as a Breakpoint does:
// /tools/j and /toolConfig/tools/j, /system[/k], /messages/i/content[/k].
func promptPosition(p string) (Breakpoint, bool) {
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	index := func(i int) (int, bool) {
		if i >= len(segs) {
			return 0, false
		}
		n, err := strconv.Atoi(segs[i])
		return n, err == nil
	}
	switch {
	case segs[0] == "tools":
		if j, ok := index(1); ok {
			return Breakpoint{Section: SectionTools, Message: -1, Block: j}, true
		}
	case segs[0] == "toolConfig" && len(segs) > 1 && segs[1] == "tools":
		if j, ok := index(2); ok {
			return Breakpoint{Section: SectionTools, Message: -1, Block: j}, true
		}
	case segs[0] == "system":
		k, ok := index(1)
		if !ok {
			k = 0 // a system string is one block
		}
		return Breakpoint{Section: SectionSystem, Message: -1, Block: k}, true
	case segs[0] == "messages":
		i, ok := index(1)
		if !ok {
			return Breakpoint{}, false
		}
		k, ok := index(3)
		if len(segs) < 3 || segs[2] != "content" {
			return Breakpoint{}, false
		}
		if !ok {
			k = 0 // string content is one block
		}
		return Breakpoint{Section: SectionMessages, Message: i, Block: k}, true
	}
	return Breakpoint{}, false
}

// NativeContextManagement reports whether the client lets the vendor manage
// the conversation's context, so a rewriter should leave the body alone:
//
//   - Anthropic (and Bedrock InvokeModel with an Anthropic body): a
//     context_management parameter, or a compaction block in a message.
//   - Bedrock Converse: context_management in additionalModelRequestFields.
//   - OpenAI Responses ("openai_responses"): a context_management
//     parameter, or a compaction item in input.
//
// Other formats, and bodies that cannot be read, report false.
func NativeContextManagement(format string, body []byte) bool {
	root, err := parseTree(body)
	if err != nil || root.kind != nodeObject {
		return false
	}
	x := newExtraction(body)
	present := func(n *node) bool { return n != nil && !x.isNull(n) }
	switch f := strings.ToLower(format); {
	case f == FormatAnthropic || f == FormatBedrock && root.get("anthropic_version") != nil:
		if present(root.get("context_management")) {
			return true
		}
		for _, m := range root.get("messages").elems() {
			for _, b := range m.get("content").elems() {
				if x.str(b.get("type")) == "compaction" {
					return true
				}
			}
		}
	case f == FormatBedrock:
		return present(root.get("additionalModelRequestFields").get("context_management"))
	case f == FormatOpenAIResponses:
		if present(root.get("context_management")) {
			return true
		}
		for _, item := range root.get("input").elems() {
			if x.str(item.get("type")) == "compaction" {
				return true
			}
		}
	}
	return false
}
