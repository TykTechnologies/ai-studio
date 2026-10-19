// Package segments splits an LLM request body into the text a guardrail
// classifies, without losing anything, and writes changes back into the body
// in place.
//
// A Segment is one string leaf of the body: a message's text, one text block
// of a multi-part message, one string of a tool result. Its Path is the JSON
// Pointer (RFC 6901) to that leaf, so Apply can replace exactly that string
// and keep the rest of the body byte for byte: tool_use / tool_calls /
// tool_call_id, images, cache_control, is_error and fields this package has
// never heard of.
//
// A Unit is what a guardrail classifies: a whole message's text, or a whole
// tool result. Its Text joins the unit's leaves (and, for a Google
// functionResponse, the response's keys, which the model also reads), so an
// injection split across two blocks is still one text. MapRewrite maps a
// rewrite of that text back onto the leaves.
//
// Model-generated tool call arguments (tool_use input, tool_calls arguments,
// functionCall args), images, signatures and encrypted payloads are never
// segments. Block or part types this package does not know are not skipped:
// their strings (except type, ids, signatures and encrypted fields) join the
// enclosing unit, and an unknown "*_tool_result" block is a tool result.
//
// Errors, and what a caller does with each:
//
//   - ErrDuplicateKey: an object has two keys a parser could read differently
//     (a repeated key, two keys equal under case folding, or a case variant of
//     a field the format defines, which Go struct decoding on the Bedrock
//     bridges would read). Refuse the request: the guardrail cannot know
//     which copy the vendor reads.
//   - ErrUnsupportedFormat (Decisions / System One, Hugging
//     Face) and ErrUnrecognisedBody (not JSON, or JSON not shaped like the
//     format, such as an embeddings body): classify Generic(body), which
//     decodes every string of a JSON body; only a body that is not JSON at
//     all is classified as raw bytes.
package segments

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"golang.org/x/text/encoding/charmap"
)

// Formats ExtractUnits understands. They are the request filter format names the
// proxy uses (the LLM vendor, or the bridge's wire format).
const (
	FormatOpenAI    = "openai"
	FormatOllama    = "ollama"
	FormatAnthropic = "anthropic"
	FormatGoogleAI  = "google_ai"
	FormatVertex    = "vertex"
)

var (
	// ErrUnsupportedFormat: ExtractUnits has no segmenter for the format.
	ErrUnsupportedFormat = errors.New("segments: unsupported format")
	// ErrUnrecognisedBody: the body is not valid JSON or not shaped like a
	// request in the format.
	ErrUnrecognisedBody = errors.New("segments: unrecognised request body")
	// ErrDuplicateKey: an object holds two keys that parsers may resolve
	// differently. It matches nativeapi.ErrDuplicateKey under errors.Is.
	ErrDuplicateKey = fmt.Errorf("segments: %w", nativeapi.ErrDuplicateKey)
)

// Segment is one string leaf of a request body.
type Segment struct {
	guardrails.Segment
	// Path is the JSON Pointer to the string leaf holding Text, for Apply.
	Path string
	// Unit is the index of the Unit the leaf belongs to.
	Unit int
}

// Unit is one text a guardrail classifies: a message's text, or one tool
// result. Index is its position among the units; Text is the detection text.
type Unit struct {
	guardrails.Segment
	// Message is the position of the message (Google: content) holding the
	// unit in the conversation array; -1 for a system prompt kept outside it
	// (Anthropic system, Google systemInstruction, Ollama generate's system).
	// Turn is the number of assistant messages before that message, whether
	// or not they carry text, so the current turn (everything the client
	// sent since the model last spoke) is the units sharing the highest Turn
	// among non-assistant units.
	Message, Turn int
	// Path is the JSON Pointer to the node holding the unit's content (a
	// message's content, a tool result's content, a functionResponse's
	// response): replacing it replaces the whole unit.
	Path string
	// Leaves are the unit's rewritable strings, in body order.
	Leaves []Segment
	// Joined is the unit's text with nothing between its leaves, for units
	// whose blocks the vendor may set side by side (text blocks of one
	// message or tool result, text parts of one message or content), so a
	// word split across two blocks is whole in one detection text. Empty
	// when the unit has one leaf or its leaves are separate fields. It is
	// for detection only; rewrites map onto Text.
	Joined string
	// Undecodable reports a document in the unit that declares a text
	// format but whose base64 bytes do not decode to UTF-8 text (or are
	// larger than maxDecodedText): the unit cannot be read in full, and the
	// runner treats it as not scanned.
	Undecodable bool

	pieces   []piece
	sep      string
	whole    wholeNode
	adjacent bool
	// removeArray and removeElem locate a tool definition in its array,
	// for RemovalEdits.
	removeArray string
	removeElem  int
}

// piece is one part of a unit's detection text: a leaf, or detection-only
// text (a key of a Google functionResponse) that cannot be rewritten.
type piece struct {
	text string
	leaf int // index into Leaves; -1 for detection-only text
}

// Edit replaces the string leaf at Path, which must still read Old, with New.
// With Value set it instead replaces whatever node is at Path with that
// JSON, and Old and New are unused.
type Edit struct {
	Path, Old, New string
	Value          []byte
}

// wholeNode says how a unit is quarantined as a whole.
type wholeNode uint8

const (
	// wholeLeaves: the unit shares its node with other units (an Anthropic
	// message's text beside its tool_result blocks, a Google content's text
	// parts beside its functionResponse parts), so each leaf is replaced.
	wholeLeaves wholeNode = iota
	// wholeString: the node at Path may be any string (a message's or a
	// tool result's content), so it is replaced by the placeholder, images
	// and all.
	wholeString
	// wholeObject: the node at Path must stay an object (a Google
	// functionResponse's response), so it becomes {"output": placeholder}.
	wholeObject
	// wholeTextBlocks: the node at Path must stay an array of content
	// blocks (a Bedrock toolResult's content), so it becomes
	// [{"text": placeholder}].
	wholeTextBlocks
	// wholeNone: the unit is never rewritten (a tool call's arguments,
	// which the model wrote and the client already ran).
	wholeNone
)

// Quarantine returns the edits that replace all of the unit's content with
// placeholder. ok is false when the unit cannot be replaced in place: it has
// detection text outside any leaf it could replace (the keys of a generic
// body), or nothing to replace.
func (u Unit) Quarantine(placeholder string) (edits []Edit, ok bool) {
	switch u.whole {
	case wholeNone:
		return nil, false
	case wholeString:
		if u.ownsLeaves() {
			return []Edit{{Path: u.Path, Value: encodeString(placeholder)}}, true
		}
	case wholeObject:
		value := append(append([]byte(`{"output":`), encodeString(placeholder)...), '}')
		return []Edit{{Path: u.Path, Value: value}}, true
	case wholeTextBlocks:
		value := append(append([]byte(`[{"text":`), encodeString(placeholder)...), '}', ']')
		return []Edit{{Path: u.Path, Value: value}}, true
	}
	if len(u.Leaves) == 0 {
		return nil, false
	}
	for _, p := range u.pieces {
		if p.leaf < 0 {
			return nil, false
		}
	}
	for _, l := range u.Leaves {
		edits = append(edits, Edit{Path: l.Path, Old: l.Text, New: placeholder})
	}
	return edits, true
}

// ownsLeaves reports whether every leaf of the unit lies under its Path.
func (u Unit) ownsLeaves() bool {
	for _, l := range u.Leaves {
		if l.Path != u.Path && !strings.HasPrefix(l.Path, u.Path+"/") {
			return false
		}
	}
	return true
}

// MapRewrite maps a rewrite of the unit's Text (a redaction) onto edits of
// its leaves. ok is false when the rewrite cannot be mapped: it changed text
// that is not a leaf (a response key) or a redaction ran across the boundary
// between two leaves.
func (u Unit) MapRewrite(rewritten string) (edits []Edit, ok bool) {
	if u.whole == wholeNone {
		return nil, false
	}
	parts := []string{rewritten}
	if len(u.pieces) > 1 {
		parts = strings.Split(rewritten, u.sep)
	}
	if len(parts) != len(u.pieces) {
		return nil, false
	}
	for i, p := range u.pieces {
		if parts[i] == p.text {
			continue
		}
		if p.leaf < 0 {
			return nil, false
		}
		edits = append(edits, Edit{Path: u.Leaves[p.leaf].Path, Old: p.text, New: parts[i]})
	}
	return edits, true
}

// Supports reports whether ExtractUnits has a segmenter for format.
func Supports(format string) bool {
	_, ok := extractors[strings.ToLower(format)]
	return ok
}

var extractors = map[string]func(x *extraction, root *node) error{
	FormatOpenAI:    extractOpenAI,
	FormatOllama:    extractOllama,
	FormatAnthropic: extractAnthropic,
	FormatGoogleAI:  extractGoogle,
	FormatVertex:    extractGoogle,
	FormatBedrock:   extractBedrock,
}

// ExtractUnits returns the units of body: each message's text and each tool
// result, in the order they open in the body, with their leaves. Empty
// strings are not leaves, and a body with no text at all returns no units
// and no error.
//
// Roles are system, user, assistant and tool. A tool result has Role tool and
// Kind tool_result whichever message carries it, with ToolCallID and, when
// the request names it, ToolName (resolved from the assistant's matching
// tool call where the result itself does not carry it). A role ExtractUnits
// does not recognise is treated as user, so its text is scanned under user
// scopes rather than skipped.
func ExtractUnits(format string, body []byte) ([]Unit, error) {
	return ExtractUnitsWith(format, body, Options{})
}

// Options selects what is extracted besides messages and tool results.
type Options struct {
	// ToolCalls adds a unit for each tool call the model made: Role
	// assistant, Kind tool_call, the tool's name and call id, and as Text
	// the call's arguments (a JSON string as the body holds it, or the raw
	// JSON of an arguments object). Each carries a Context with the latest
	// user text before the call and the system prompt. Tool call units are
	// never rewritten: Quarantine and MapRewrite refuse them.
	ToolCalls bool
	// ToolDefinitions adds a unit for each tool the request offers the
	// model (see definitions.go), after the conversation's units.
	ToolDefinitions bool
}

// ExtractUnitsWith is ExtractUnits with opts.
func ExtractUnitsWith(format string, body []byte, opts Options) ([]Unit, error) {
	x, err := extract(format, body, opts)
	if err != nil {
		return nil, err
	}
	return x.result(), nil
}

// Generic is the fallback for a JSON body no format segmenter reads: one
// user unit holding every string of the body, decoded, with the object keys
// as detection-only text. Message is 0, so it is the current turn.
func Generic(body []byte) ([]Unit, error) {
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	x := newExtraction(body)
	x.enter(0)
	u := x.open("", wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
	x.walk(u, root, "", true, nil)
	x.finish()
	return x.result(), nil
}

// Read returns the units of a request body in format the way a guardrail
// reads it: with the format's segmenter, or, for a JSON body no segmenter
// reads (another format, another shape), Generic. Only a body that is not
// JSON (ErrUnrecognisedBody) or has a repeated or case-variant key
// (ErrDuplicateKey) is an error.
func Read(format string, body []byte) ([]Unit, error) {
	return ReadWith(format, body, Options{})
}

// ReadWith is Read with opts. A body read generically has no tool calls.
func ReadWith(format string, body []byte, opts Options) ([]Unit, error) {
	if Supports(format) {
		units, err := ExtractUnitsWith(format, body, opts)
		if err == nil || errors.Is(err, ErrDuplicateKey) {
			return units, err
		}
	}
	return Generic(body)
}

// CheckKeys reports a repeated key anywhere in a JSON body as ErrDuplicateKey.
// A body that is not JSON returns ErrUnrecognisedBody.
func CheckKeys(body []byte) error {
	_, err := parseTree(body)
	return err
}

func extract(format string, body []byte, opts Options) (*extraction, error) {
	fn, ok := extractors[strings.ToLower(format)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	if root.kind != nodeObject {
		return nil, fmt.Errorf("%w: not a JSON object", ErrUnrecognisedBody)
	}
	x := newExtraction(body)
	x.opts = opts
	if err := fn(x, root); err != nil {
		return nil, err
	}
	if err := x.definitions(strings.ToLower(format), root); err != nil {
		return nil, err
	}
	x.finish()
	if opts.ToolCalls {
		x.attachGoals()
	}
	return x, nil
}

// extraction is the state of one ExtractUnits call.
type extraction struct {
	body []byte
	opts Options
	// names maps tool call ids to tool names from every call the request
	// shows the model making.
	names map[string]string
	// msg and turn are stamped on each unit opened; the format's message
	// loop advances them.
	msg, turn int
	units     []*Unit
	// leaves are every unit's leaves in body order; finish numbers them.
	leaves []Segment
}

func newExtraction(body []byte) *extraction {
	return &extraction{body: body, names: map[string]string{}, msg: -1}
}

// enter starts message i of the conversation array.
func (x *extraction) enter(i int) { x.msg = i }

// leave ends the current message; an assistant message starts a new turn.
func (x *extraction) leave(role string) {
	if role == guardrails.RoleAssistant {
		x.turn++
	}
}

// open starts a unit whose content is the node at path, quarantined as whole
// says.
func (x *extraction) open(path string, whole wholeNode, meta guardrails.Segment) *Unit {
	if meta.Kind == "" {
		meta.Kind = guardrails.KindText
	}
	meta.Index = len(x.units)
	u := &Unit{Segment: meta, Message: x.msg, Turn: x.turn, Path: path, whole: whole}
	x.units = append(x.units, u)
	return u
}

// leaf adds the string n at path to u. Empty strings and non-strings are
// not leaves.
func (x *extraction) leaf(u *Unit, n *node, path string) {
	text, ok := n.text(x.body)
	if !ok || text == "" {
		return
	}
	s := Segment{Segment: u.Segment, Path: path, Unit: u.Index}
	s.Text = text
	u.pieces = append(u.pieces, piece{text: text, leaf: len(u.Leaves)})
	u.Leaves = append(u.Leaves, s)
	x.leaves = append(x.leaves, s)
}

// detect adds detection-only text to u.
func (x *extraction) detect(u *Unit, text string) {
	if text != "" {
		u.pieces = append(u.pieces, piece{text: text, leaf: -1})
	}
}

// maxDecodedText bounds the text decoded from one base64 document.
const maxDecodedText = 16 << 20

// decodedText adds the text of a base64 document in a text format (plain
// text, Markdown, HTML, CSV) to u as detection text: the model reads the
// text, the body holds base64, so it can be classified but not rewritten in
// place. Padded and unpadded base64 are read; anything else, or a document
// over maxDecodedText, marks the unit Undecodable.
//
// Bytes that are not valid UTF-8 are read twice: as UTF-8 with the invalid
// sequences replaced, as a vendor reads them (so one stray byte cannot turn
// a UTF-8 document into mojibake the classifier misreads), and as
// Windows-1252, the usual encoding of legacy text exports.
func (x *extraction) decodedText(u *Unit, n *node) {
	s := x.str(n)
	if s == "" {
		return
	}
	if base64.StdEncoding.DecodedLen(len(s)) > maxDecodedText {
		u.Undecodable = true
		return
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		u.Undecodable = true
		return
	}
	if utf8.Valid(b) {
		x.detect(u, string(b))
		return
	}
	x.detect(u, strings.ToValidUTF8(string(b), "\uFFFD"))
	if cp1252, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
		x.detect(u, string(cp1252))
	}
}

// walk adds every string inside n to u, at any depth. With keys, object keys
// are detection text too (tool output, where the keys are the tool's data).
// skip names members that are not text a model reads.
func (x *extraction) walk(u *Unit, n *node, path string, keys bool, skip func(string) bool) {
	if n == nil {
		return
	}
	switch n.kind {
	case nodeString:
		x.leaf(u, n, path)
	case nodeObject:
		for i, k := range n.keys {
			if skip != nil && skip(k) {
				continue
			}
			if keys {
				x.detect(u, k)
			}
			x.walk(u, n.vals[i], pointer(path, k), keys, skip)
		}
	case nodeArray:
		for i, v := range n.vals {
			x.walk(u, v, pointerIndex(path, i), keys, skip)
		}
	}
}

// opaque names members of an unknown block that are identifiers, signatures,
// encrypted payloads or metadata rather than text.
func opaque(key string) bool {
	return key == "type" || key == "id" || strings.HasSuffix(key, "_id") ||
		key == "signature" || strings.HasPrefix(key, "encrypted_") || key == "page_age"
}

// finish drops units with no text, numbers units and leaves, and builds each
// unit's detection text.
func (x *extraction) finish() {
	kept := x.units[:0]
	renumber := make(map[int]int, len(x.units))
	for _, u := range x.units {
		if len(u.pieces) == 0 && !u.Undecodable {
			continue
		}
		renumber[u.Index] = len(kept)
		u.Index = len(kept)
		u.buildText()
		kept = append(kept, u)
	}
	x.units = kept
	for i := range x.leaves {
		x.leaves[i].Index = i
		x.leaves[i].Unit = renumber[x.leaves[i].Unit]
	}
	// The units' copies of their leaves carry the final numbering too.
	next := make([]int, len(x.units))
	for _, s := range x.leaves {
		u := x.units[s.Unit]
		u.Leaves[next[s.Unit]] = s
		next[s.Unit]++
	}
}

// buildText joins the unit's pieces with a separator that occurs nowhere
// else in the result, so MapRewrite can split a rewrite back into pieces.
// The separator is whitespace (a newline, form feeds, a newline): patterns
// that allow whitespace between words still match across two blocks.
func (u *Unit) buildText() {
	if len(u.pieces) == 0 {
		return
	}
	if len(u.pieces) == 1 {
		u.Text = u.pieces[0].text
		return
	}
	texts := make([]string, len(u.pieces))
	for i, p := range u.pieces {
		texts[i] = p.text
	}
	if u.adjacent && len(u.Leaves) == len(u.pieces) {
		// Only blocks of text sit side by side; a walked unknown part's
		// keys do not.
		u.Joined = strings.Join(texts, "")
	}
	for k := 1; ; k++ {
		sep := "\n" + strings.Repeat("\f", k) + "\n"
		joined := strings.Join(texts, sep)
		if splitsBack(joined, sep, texts) {
			u.sep, u.Text = sep, joined
			return
		}
	}
}

func splitsBack(joined, sep string, texts []string) bool {
	parts := strings.Split(joined, sep)
	if len(parts) != len(texts) {
		return false
	}
	for i := range parts {
		if parts[i] != texts[i] {
			return false
		}
	}
	return true
}

func (x *extraction) result() []Unit {
	out := make([]Unit, len(x.units))
	for i, u := range x.units {
		out[i] = *u
	}
	return out
}

// str is the text of a string node, "" for anything else.
func (x *extraction) str(n *node) string {
	s, _ := n.text(x.body)
	return s
}

func (x *extraction) isNull(n *node) bool {
	return n == nil || n.kind == nodeScalar && string(x.body[n.start:n.end]) == "null"
}

// objects returns the elements of the array n, which must all be objects. A
// missing or null value is an empty list; anything else is a shape error.
func (x *extraction) objects(n *node, path string) ([]*node, error) {
	if x.isNull(n) {
		return nil, nil
	}
	if n.kind != nodeArray {
		return nil, shapeError(path, "an array")
	}
	for i, v := range n.vals {
		if v.kind != nodeObject {
			return nil, shapeError(pointerIndex(path, i), "an object")
		}
	}
	return n.vals, nil
}

// required is objects for the member the format is built around (messages,
// contents): without it the body is some other request.
func (x *extraction) required(root *node, key string) ([]*node, error) {
	n := root.get(key)
	if n == nil {
		return nil, fmt.Errorf("%w: no %q", ErrUnrecognisedBody, key)
	}
	if n.kind != nodeArray {
		return nil, shapeError(pointer("", key), "an array")
	}
	return x.objects(n, pointer("", key))
}

// strict checks an object the extractor reads: no two of its keys may be
// equal under Unicode case folding, and no key may be a case variant of one
// of fields. Go's encoding/json matches struct fields case-insensitively, so
// on the paths that decode the filtered body into structs (the Bedrock
// bridges) "Content" would be read where the guardrail saw nothing.
func (x *extraction) strict(n *node, path string, fields ...string) error {
	if !n.is(nodeObject) {
		return nil
	}
	seen := make(map[string]string, len(n.keys))
	for _, k := range n.keys {
		f := nativeapi.FoldKey(k)
		if prev, dup := seen[f]; dup {
			return fmt.Errorf("%w: case-variant keys%s at %s", ErrDuplicateKey, keyName(prev)+keyName(k), maskPath(path))
		}
		seen[f] = k
	}
	for _, field := range fields {
		if k, ok := seen[nativeapi.FoldKey(field)]; ok && k != field {
			return fmt.Errorf("%w: a case variant of%s at %s", ErrDuplicateKey, keyName(field), maskPath(path))
		}
	}
	return nil
}

// alias returns the member spelt either way (proto JSON accepts the field
// name and its lowerCamelCase form); both at once is a duplicate.
func (x *extraction) alias(n *node, path, camel, snake string) (*node, string, error) {
	a, b := n.get(camel), n.get(snake)
	switch {
	case a != nil && b != nil:
		return nil, "", fmt.Errorf("%w: both%s and%s at %s", ErrDuplicateKey, keyName(camel), keyName(snake), maskPath(path))
	case b != nil:
		return b, snake, nil
	}
	return a, camel, nil
}

// noteCall records a tool call the model made, for naming its result.
func (x *extraction) noteCall(id, name *node) {
	if i, n := x.str(id), x.str(name); i != "" && n != "" {
		x.names[i] = n
	}
}

// toolCall opens a unit for a tool call the model made, when the extraction
// asks for them: args is the arguments node at path (a JSON string, or an
// object such as Anthropic's input), id and name the call's. A string is the
// unit's leaf; an object is its raw JSON, as detection text. A call with no
// arguments, or empty ones, still names the tool, so it is a unit with the
// text "{}".
func (x *extraction) toolCall(args *node, path string, id, name *node) {
	if !x.opts.ToolCalls {
		return
	}
	u := x.open(path, wholeNone, guardrails.Segment{
		Role: guardrails.RoleAssistant, Kind: guardrails.KindToolCall,
		ToolName: x.str(name), ToolCallID: x.str(id),
	})
	switch {
	case args.is(nodeString) && x.str(args) != "":
		x.leaf(u, args, path)
	case args.is(nodeObject) || args.is(nodeArray):
		x.detect(u, string(x.body[args.start:args.end]))
	}
	if len(u.pieces) == 0 {
		x.detect(u, "{}")
	}
}

// attachGoals sets the Context of every tool call unit: the latest user text
// before the call's message, and the system prompt, each clipped
// (guardrails.ClipContext).
func (x *extraction) attachGoals() {
	var system []string
	for _, u := range x.units {
		if u.Role == guardrails.RoleSystem && u.Kind == guardrails.KindText && u.Text != "" {
			system = append(system, u.Text)
		}
	}
	sys := guardrails.ClipContext(strings.Join(system, "\n\n"))
	// Units are in body order, so the user text seen last before a call is
	// the user's latest instruction to the model that made it.
	lastUser := ""
	for _, u := range x.units {
		switch {
		case u.Role == guardrails.RoleUser && u.Kind == guardrails.KindText && strings.TrimSpace(u.Text) != "":
			lastUser = u.Text
		case u.IsToolCall():
			u.Context = &guardrails.SegmentContext{UserGoal: guardrails.ClipContext(lastUser), System: sys}
		}
	}
}

// toolResult is the unit shape of a tool result answering call id.
// explicitName is the name the result itself carries, if any; otherwise the
// name comes from the matching call.
func (x *extraction) toolResult(id, explicitName string) guardrails.Segment {
	name := explicitName
	if name == "" {
		name = x.names[id]
	}
	return guardrails.Segment{Role: guardrails.RoleTool, Kind: guardrails.KindToolResult, ToolName: name, ToolCallID: id}
}

func shapeError(path, want string) error {
	return fmt.Errorf("%w: %s is not %s", ErrUnrecognisedBody, maskPath(path), want)
}

// messageRole maps a chat message role onto a segment role. Unknown roles
// (including OpenAI's developer) are user, as the message extractor has
// always treated them, so their text stays under the default scopes.
func messageRole(role string) string {
	switch role {
	case "system":
		return guardrails.RoleSystem
	case "assistant":
		return guardrails.RoleAssistant
	case "tool", "function":
		return guardrails.RoleTool
	}
	return guardrails.RoleUser
}
