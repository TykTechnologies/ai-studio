package extract

import (
	"bytes"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// ParseRequestExtracts is ParseRequest for what the capture keeps
// (Request.Apply: the trailing tool results, the previous response id and
// the offered tools). A body with no tool key at all, which is most chat
// requests, returns at once instead of paying for the full parse (bead
// jyx.149: the parse costs about 2.5 us per KiB of body). LastRole is not
// set on that path. The Responses format is always parsed: its
// previous_response_id needs no tool.
func ParseRequestExtracts(format string, body []byte, fp agenttrace.Fingerprinter) (Request, error) {
	if !strings.EqualFold(format, FormatOpenAIResponses) && !hasToolKeys(body) {
		return Request{}, nil
	}
	return ParseRequest(format, body, fp)
}

// hasToolKeys reports whether body has a JSON string starting "tool" or
// "function": every key a tool result with a call id or a tool definition
// sits under (tool_call_id, tool_use_id, toolUseId, functionResponse,
// tools, toolConfig, functions) does. One pass over the body's quotes, so
// long text without quotes is skipped at memchr speed.
func hasToolKeys(body []byte) bool {
	for {
		i := bytes.IndexByte(body, '"')
		if i < 0 {
			return false
		}
		body = body[i+1:]
		if bytes.HasPrefix(body, []byte("tool")) || bytes.HasPrefix(body, []byte("function")) {
			return true
		}
	}
}
