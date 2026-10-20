package agenttrace

import (
	"sort"
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

// Attributes a gateway capture writes, and the values each may hold. The
// hub keeps only these from an edge event (jyx.160, jyx.167): every other
// tyk.* attribute is the hub's own statement (the correlator's attribution,
// the offered tool set), so an edge must not be able to preset it, and
// content never travels in attributes.
var captureAttributes = map[string]func(string) bool{
	"mcp.method.name":       func(v string) bool { return printableUpTo(v, 128) },
	"mcp.protocol.version":  func(v string) bool { return printableUpTo(v, 32) },
	"http.request.method":   isHTTPMethod,
	"tyk.server_tool_calls": isDigits,
	"tyk.content.withheld":  func(v string) bool { return withheldReasons[v] },
}

// WithheldEdgeLinkUnverifiedTLS is the tyk.content.withheld reason of an
// edge whose link to the hub is not verified TLS (jyx.164).
const WithheldEdgeLinkUnverifiedTLS = "edge_link_unverified_tls"

var withheldReasons = map[string]bool{WithheldEdgeLinkUnverifiedTLS: true}

// CaptureAttributeKeys lists the attributes a capture may carry (tests pin
// it).
func CaptureAttributeKeys() []string {
	keys := make([]string, 0, len(captureAttributes))
	for k := range captureAttributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// boundAttributes keeps the attributes a capture writes, with valid values.
func boundAttributes(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	var out map[string]string
	for k, v := range in {
		if ok := captureAttributes[k]; ok != nil && ok(v) {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	return out
}

func printableUpTo(v string, n int) bool {
	if v == "" || len(v) > n {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return false
		}
	}
	return true
}

func isHTTPMethod(v string) bool {
	if v == "" || len(v) > 16 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 'A' || v[i] > 'Z' {
			return false
		}
	}
	return true
}

func isDigits(v string) bool {
	if v == "" || len(v) > 9 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
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
