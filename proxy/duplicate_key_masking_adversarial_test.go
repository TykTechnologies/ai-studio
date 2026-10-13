//go:build enterprise

package proxy

// Adversarial tests for 4uc.24 (tests agent): a request refused for
// duplicate or case-variant keys must not carry the key text of tool data
// into the client error or the compliance events. Keys inside tool output
// are data: credentials, e-mail addresses, record ids.

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

func fixtureOf(format, body string) toolfixtures.Fixture {
	return toolfixtures.Fixture{Name: "inline", Format: format, Body: []byte(body)}
}

func TestAdversarial_DuplicateKeyRefusalMasksToolDataKeys(t *testing.T) {
	p := newAllFormatsFilterProxy()
	sink := installComplianceSink(t)
	const secret = "AKIAIOSFODNN7EXAMPLE"
	cases := []struct {
		name, format, body string
		leaks              []string
		// mustRefuse: an exact repeated key is refused anywhere; a case
		// variant only in objects a segmenter reads, not inside tool data.
		mustRefuse bool
	}{
		{"gemini response exact duplicate", "google_ai",
			`{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"vault","response":{"` + secret + `":1,"` + secret + `":2}}}]}]}`,
			[]string{secret}, true},
		{"gemini response case variant", "google_ai",
			`{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"vault","response":{"keys":{"` + secret + `":1,"` + strings.ToLower(secret) + `":2}}}}]}]}`,
			[]string{secret, strings.ToLower(secret)}, false},
		{"anthropic unknown tool result block", "anthropic",
			`{"model":"m","max_tokens":1,"messages":[{"role":"user","content":[{"type":"custom_tool_result","tool_use_id":"t","content":{"jane.doe@example.com":"a","JANE.DOE@example.com":"b"}}]}]}`,
			[]string{"jane.doe@example.com", "JANE.DOE"}, false},
		{"generic body (bedrock)", "bedrock",
			`{"messages":[{"role":"user","content":[{"toolResult":{"toolUseId":"t","content":[{"json":{"` + secret + `":1,"` + secret + `":2}}]}}]}]}`,
			[]string{secret}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sink.take()
			f := builtinGuardrail("log-injection", guardrails.CategoryInjection, guardrails.ActionLog, guardrails.ScopeAllMessages)
			_, err := runFixture(t, p, fixtureOf(c.format, c.body), f)
			if err == nil && c.mustRefuse {
				t.Fatal("a body with a repeated key was not refused")
			}
			var texts []string
			if err != nil {
				texts = append(texts, err.Error())
			}
			for _, e := range sink.take() {
				texts = append(texts, e.Description, e.Metadata, e.EventType)
			}
			for _, txt := range texts {
				for _, leak := range c.leaks {
					if strings.Contains(strings.ToLower(txt), strings.ToLower(leak)) {
						t.Errorf("tool-data key %q leaked into %q", leak, txt)
					}
				}
			}
		})
	}
}
