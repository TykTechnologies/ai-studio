package agenttrace

import "unicode/utf8"

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
