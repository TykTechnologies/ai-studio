package extract

import "unicode/utf8"

// Bounds on what one body contributes to an event (jyx.62). Bodies are
// untrusted, and extracts travel in the capture proto, the edge's 4 MiB
// pulse chunks and the span rows.
const (
	// MaxIDBytes: a longer tool call or response id is not a real one. It is
	// dropped (left empty, so unlinkable) rather than cut, since two cut ids
	// could collide into a false R1 link.
	MaxIDBytes = 256
	// MaxNameBytes caps tool names; longer ones are cut.
	MaxNameBytes = 256
	// MaxToolCalls caps the tool calls, server tool calls and tool results
	// kept per body.
	MaxToolCalls = 512
	// MaxFinishReasons caps finish reasons (one per choice), each cut to
	// maxReasonBytes.
	MaxFinishReasons = 16
	maxReasonBytes   = 64
)

var knownRoles = map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}

func boundID(s string) string {
	if len(s) > MaxIDBytes {
		return ""
	}
	return s
}

func boundText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func boundCalls(calls []ToolCall) []ToolCall {
	if len(calls) > MaxToolCalls {
		calls = calls[:MaxToolCalls]
	}
	for i := range calls {
		calls[i].ID = boundID(calls[i].ID)
		calls[i].Name = boundText(calls[i].Name, MaxNameBytes)
	}
	return calls
}

func (r *Response) bound() {
	r.ID = boundID(r.ID)
	r.ToolCalls = boundCalls(r.ToolCalls)
	r.ServerToolCalls = boundCalls(r.ServerToolCalls)
	if len(r.FinishReasons) > MaxFinishReasons {
		r.FinishReasons = r.FinishReasons[:MaxFinishReasons]
	}
	for i := range r.FinishReasons {
		r.FinishReasons[i] = boundText(r.FinishReasons[i], maxReasonBytes)
	}
}

func (r *Request) bound() {
	r.PreviousResponseID = boundID(r.PreviousResponseID)
	if len(r.ToolResults) > MaxToolCalls {
		r.ToolResults = r.ToolResults[len(r.ToolResults)-MaxToolCalls:]
	}
	for i := range r.ToolResults {
		r.ToolResults[i].ID = boundID(r.ToolResults[i].ID)
		r.ToolResults[i].Name = boundText(r.ToolResults[i].Name, MaxNameBytes)
	}
	if r.LastRole != "" && !knownRoles[r.LastRole] {
		r.LastRole = "other"
	}
}
