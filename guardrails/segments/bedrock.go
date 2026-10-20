package segments

import (
	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// FormatBedrock is the native /llm/ pass-through to Amazon Bedrock: a
// Converse request, or an InvokeModel request carrying an Anthropic model's
// own body (anthropic_version set).
const FormatBedrock = "bedrock"

var (
	bedrockRootFields       = []string{"messages", "system", "anthropic_version", "promptVariables", "additionalModelRequestFields"}
	bedrockMessageFields    = []string{"role", "content"}
	bedrockBlockFields      = []string{"text", "image", "document", "video", "toolUse", "toolResult", "guardContent", "reasoningContent", "cachePoint", "citationsContent"}
	bedrockToolUseFields    = []string{"toolUseId", "name", "input"}
	bedrockToolResultFields = []string{"toolUseId", "content", "status"}
	bedrockResultFields     = []string{"text", "json", "image", "document", "video"}
)

// bedrockSilent are the content block members that carry no text a
// guardrail reads: model reasoning and cache markers. Media blocks
// (image, document, video) are read for their text and skip only their
// binary sources (bedrockBinary): a document can carry its text inline.
var bedrockSilent = map[string]bool{
	"reasoningContent": true, "cachePoint": true, "citationsContent": true,
}

// bedrockTextFormats are the document formats whose bytes are text.
var bedrockTextFormats = map[string]bool{"txt": true, "md": true, "html": true, "csv": true}

// bedrockDocumentBytes reads the bytes of a document in a text format.
func (x *extraction) bedrockDocumentBytes(u *Unit, doc *node) {
	if bedrockTextFormats[x.str(doc.get("format"))] {
		x.decodedText(u, doc.get("source").get("bytes"))
	}
}

// bedrockBinary names members of a media block that hold bytes or a
// storage location rather than text.
func bedrockBinary(key string) bool {
	return key == "bytes" || key == "s3Location" || key == "format" || opaque(key)
}

// Bedrock Converse: system[] and messages[].content[] hold blocks, each an
// object with one member naming its kind ({"text": ...}, {"toolUse": ...}).
// A toolResult answers a toolUse through toolUseId; its content blocks are
// text or json, and a json block is read like a Google functionResponse:
// every string, with its keys as detection text.
func extractBedrock(x *extraction, root *node) error {
	if root.get("anthropic_version") != nil {
		return extractAnthropic(x, root)
	}
	if err := x.strict(root, "", bedrockRootFields...); err != nil {
		return err
	}
	msgs, err := x.required(root, "messages")
	if err != nil {
		return err
	}
	for i, m := range msgs {
		path := pointerIndex("/messages", i)
		if err := x.strict(m, path, bedrockMessageFields...); err != nil {
			return err
		}
		for _, b := range m.get("content").elems() {
			if use := b.get("toolUse"); use.is(nodeObject) {
				x.noteCall(use.get("toolUseId"), use.get("name"))
			}
		}
	}
	system := x.open("/system", wholeLeaves, guardrails.Segment{Role: guardrails.RoleSystem})
	system.adjacent = true
	if err := x.bedrockBlocks(system, root.get("system"), "/system"); err != nil {
		return err
	}
	for i, m := range msgs {
		x.enter(i)
		path := pointer(pointerIndex("/messages", i), "content")
		role := guardrails.RoleUser
		if x.str(m.get("role")) == "assistant" {
			role = guardrails.RoleAssistant
		}
		u := x.open(path, wholeLeaves, guardrails.Segment{Role: role})
		u.adjacent = true
		if err := x.bedrockBlocks(u, m.get("content"), path); err != nil {
			return err
		}
		x.leave(role)
	}
	// Prompt variables (a prompt-management modelId) are substituted into
	// the prompt the model reads; model-specific request fields may carry
	// text too. Both belong to the current turn.
	vars := x.open("/promptVariables", wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
	x.walk(vars, root.get("promptVariables"), "/promptVariables", false, opaque)
	extra := x.open("/additionalModelRequestFields", wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
	x.walk(extra, root.get("additionalModelRequestFields"), "/additionalModelRequestFields", true, nil)
	return nil
}

// bedrockBlocks adds an array of content blocks to u, opening a unit for
// each tool result.
func (x *extraction) bedrockBlocks(u *Unit, n *node, path string) error {
	blocks, err := x.objects(n, path)
	if err != nil {
		return err
	}
	for k, b := range blocks {
		bp := pointerIndex(path, k)
		if err := x.strict(b, bp, bedrockBlockFields...); err != nil {
			return err
		}
		for i, key := range b.keys {
			val, memberPath := b.vals[i], pointer(bp, key)
			switch {
			case key == "text":
				x.leaf(u, val, memberPath)
			case key == "guardContent":
				x.walk(u, val, memberPath, false, opaque)
			case key == "toolUse":
				if err := x.strict(val, memberPath, bedrockToolUseFields...); err != nil {
					return err
				}
				x.toolCall(val.get("input"), pointer(memberPath, "input"), val.get("toolUseId"), val.get("name"))
			case key == "toolResult":
				if err := x.bedrockToolResult(val, memberPath); err != nil {
					return err
				}
			case key == "image" || key == "document" || key == "video":
				x.walk(u, val, memberPath, false, bedrockBinary)
				if key == "document" {
					x.bedrockDocumentBytes(u, val)
				}
			case bedrockSilent[key]:
			default:
				x.walk(u, val, memberPath, true, opaque)
			}
		}
	}
	return nil
}

func (x *extraction) bedrockToolResult(tr *node, path string) error {
	if err := x.strict(tr, path, bedrockToolResultFields...); err != nil {
		return err
	}
	contentPath := pointer(path, "content")
	result := x.open(contentPath, wholeTextBlocks, x.toolResult(x.str(tr.get("toolUseId")), ""))
	result.adjacent = true
	blocks, err := x.objects(tr.get("content"), contentPath)
	if err != nil {
		return err
	}
	for k, b := range blocks {
		bp := pointerIndex(contentPath, k)
		if err := x.strict(b, bp, bedrockResultFields...); err != nil {
			return err
		}
		for i, key := range b.keys {
			val, memberPath := b.vals[i], pointer(bp, key)
			switch {
			case key == "text":
				x.leaf(result, val, memberPath)
			case key == "json":
				x.walk(result, val, memberPath, true, nil)
			case key == "image" || key == "document" || key == "video":
				x.walk(result, val, memberPath, false, bedrockBinary)
				if key == "document" {
					x.bedrockDocumentBytes(result, val)
				}
			case bedrockSilent[key]:
			default:
				x.walk(result, val, memberPath, true, opaque)
			}
		}
	}
	return nil
}
