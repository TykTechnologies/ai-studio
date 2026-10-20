package agenttrace

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Extract bounds, the same as the extraction library applies at capture
// (pkg/agenttrace/extract, jyx.62). The hub applies them again to an edge's
// events at ingest (Extracts.Bound): edges share credentials, so what an
// edge sends is not trusted to be bounded (jyx.159).
const (
	maxExtractIDBytes     = 256
	maxExtractNameBytes   = 255
	maxExtractCalls       = 512
	maxExtractReasons     = 16
	maxExtractReasonBytes = 64
	maxExtractOffered     = 256
)

// Bound applies the extract bounds: over-long ids are dropped (a cut id
// could link falsely), names are cut, and the lists are capped.
func (x *Extracts) Bound() {
	x.ResponseID = boundID(x.ResponseID)
	x.PreviousResponseID = boundID(x.PreviousResponseID)
	x.ToolCallID = boundID(x.ToolCallID)
	if len(x.ToolCalls) > maxExtractCalls {
		x.ToolCalls = x.ToolCalls[:maxExtractCalls]
	}
	for i := range x.ToolCalls {
		x.ToolCalls[i].ID = boundID(x.ToolCalls[i].ID)
		x.ToolCalls[i].Name = cutText(x.ToolCalls[i].Name, maxExtractNameBytes)
	}
	if len(x.ToolResults) > maxExtractCalls {
		x.ToolResults = x.ToolResults[len(x.ToolResults)-maxExtractCalls:]
	}
	for i := range x.ToolResults {
		x.ToolResults[i].ID = boundID(x.ToolResults[i].ID)
	}
	if len(x.FinishReasons) > maxExtractReasons {
		x.FinishReasons = x.FinishReasons[:maxExtractReasons]
	}
	for i := range x.FinishReasons {
		x.FinishReasons[i] = cutText(x.FinishReasons[i], maxExtractReasonBytes)
	}
	if len(x.OfferedTools) > maxExtractOffered {
		x.OfferedTools = x.OfferedTools[:maxExtractOffered]
	}
	for i := range x.OfferedTools {
		x.OfferedTools[i] = cutText(x.OfferedTools[i], maxExtractNameBytes)
	}
}

// Attribute bounds for an edge event's Attributes (jyx.160).
const (
	maxAttributes       = 64
	maxAttributeKey     = 128
	maxAttributeValue   = 1024
	attributeNamespaces = "tyk. gen_ai. mcp. http."
)

// boundAttributes keeps an edge event's attributes in the namespaces the
// gateway writes (tyk.*, gen_ai.*, mcp.*, http.*), with bounded keys and
// values and at most maxAttributes of them (in key order, so the choice is
// stable). Attributes go to an unbounded column; an edge is not trusted to
// have bounded them.
func boundAttributes(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		if len(k) <= maxAttributeKey && inAttributeNamespace(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > maxAttributes {
		keys = keys[:maxAttributes]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = cutText(in[k], maxAttributeValue)
	}
	return out
}

func inAttributeNamespace(k string) bool {
	for _, ns := range strings.Fields(attributeNamespaces) {
		if strings.HasPrefix(k, ns) {
			return true
		}
	}
	return false
}

func boundID(s string) string {
	if len(s) > maxExtractIDBytes {
		return ""
	}
	return s
}

func cutText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
