package extract

import (
	"bytes"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// toolResultMarkers are the keys a tool result with a call id carries, per
// format. A body without its format's key holds no result R1 could link.
var toolResultMarkers = map[string][]byte{
	FormatOpenAI:    []byte(`tool_call_id`),
	FormatOllama:    []byte(`tool_call_id`),
	FormatAnthropic: []byte(`tool_use_id`),
	FormatGoogleAI:  []byte(`functionResponse`),
	FormatVertex:    []byte(`functionResponse`),
	FormatBedrock:   []byte(`toolUseId`),
}

// ParseRequestExtracts is ParseRequest for what the capture keeps
// (Request.Apply: the trailing tool results and the previous response id).
// A body with no tool result key for its format, which is most chat
// requests, returns at once instead of paying for the full parse (bead
// jyx.149: the parse costs about 2.5 us per KiB of body). LastRole is not
// set on that path.
func ParseRequestExtracts(format string, body []byte, fp agenttrace.Fingerprinter) (Request, error) {
	if marker, ok := toolResultMarkers[strings.ToLower(format)]; ok && !bytes.Contains(body, marker) {
		return Request{}, nil
	}
	return ParseRequest(format, body, fp)
}
