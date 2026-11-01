package proxy

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
)

// The caller's X-Tyk-Prompt-Template header (templates.HintHeader) names the
// trusted prompt template its prompt was built from. It is a hint to the
// request guardrails: it crosses the /ai/ -> /llm/call/ loopback, where the
// request filters run, and never reaches a vendor.

// stripPromptTemplateHint removes the hint before a request leaves for the
// vendor. Keys are compared case-insensitively: a header set by direct map
// assignment may not be canonical.
func stripPromptTemplateHint(h http.Header) {
	for k := range h {
		if strings.EqualFold(k, templates.HintHeader) {
			delete(h, k)
		}
	}
}
