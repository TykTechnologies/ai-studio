package proxy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// hdrGuardrailReason carries a guardrail block's reason code on the response.
const hdrGuardrailReason = "X-Tyk-AI-Guardrail-Reason"

// PolicyBlockError is a request a filter blocked. Its text is what the client
// reads, on every path that only carries a message (bridges, Bedrock, the
// Anthropic shim): the filter's message, then, for a guardrail, why (a
// stable code and a sentence naming the cause and where it was, never what
// was matched) and the request id to quote to an administrator.
type PolicyBlockError struct {
	Filter    string
	Message   string
	Code      string
	Reason    string
	RequestID string
}

func (e *PolicyBlockError) Error() string {
	return "Policy error: " + e.Filter + " - " + explainBlock(e.Message, e.Code, e.Reason, e.RequestID)
}

// explainBlock appends a guardrail's reason and the request id to a block
// message.
func explainBlock(msg, code, reason, id string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(msg))
	if reason != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), ".") {
			b.WriteString(".")
		}
		b.WriteString(" Reason: ")
		b.WriteString(reason)
		if code != "" {
			b.WriteString(" (" + code + ")")
		}
		b.WriteString(".")
	}
	if id != "" && (reason != "" || code != "") {
		b.WriteString(" Request ID: " + id + ".")
	}
	return strings.TrimSpace(b.String())
}

// newPolicyBlock builds the error for a request filter's block.
func newPolicyBlock(r *http.Request, filterName string, out *scripting.ScriptOutput) *PolicyBlockError {
	msg := out.Message
	if msg == "" {
		msg = "blocked by policy"
	}
	id := ""
	if out.ReasonCode != "" {
		id = requestid.FromContext(r.Context())
	}
	return &PolicyBlockError{Filter: filterName, Message: msg, Code: out.ReasonCode, Reason: out.Reason, RequestID: id}
}

// responseBlockMessage is a response filter's block message for the client,
// with a guardrail's reason and the request id.
func responseBlockMessage(r *http.Request, msg string, out *scripting.ScriptOutput) string {
	if out == nil || out.ReasonCode == "" {
		return msg
	}
	return explainBlock(msg, out.ReasonCode, out.Reason, requestid.FromContext(r.Context()))
}

// policyBlockOf returns the PolicyBlockError in err's chain, if any.
func policyBlockOf(err error) *PolicyBlockError {
	var pb *PolicyBlockError
	if errors.As(err, &pb) {
		return pb
	}
	return nil
}
