//go:build enterprise

package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Filters run top to bottom in the order stored on the LLM and the first
// block wins, so the rejection names whichever blocking filter the admin
// placed first. Swapping the chain swaps the name.
func TestRunRequestFilters_ChainOrderFirstBlockWins(t *testing.T) {
	p := newFilterTestProxy()

	blockAs := func(name string) *models.Filter {
		return &models.Filter{
			Name:   name,
			Script: []byte(`output := {block: true, message: "stop"}`),
		}
	}
	first, second := blockAs("first-block"), blockAs("second-block")

	run := func(filters ...*models.Filter) error {
		llm := &models.LLM{Vendor: models.OPENAI, Filters: filters}
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
		r := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(body))
		_, err := p.runRequestFilters(llm, r, []byte(body), "openai", "gpt-4")
		return err
	}

	err := run(first, second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "first-block")
	require.NotContains(t, err.Error(), "second-block")

	err = run(second, first)
	require.Error(t, err)
	require.Contains(t, err.Error(), "second-block")
	require.NotContains(t, err.Error(), "first-block")
}

// One set of runners, built once, serves many bodies: the batch-create path
// builds them per batch and runs them for every item. Each run sees its own
// body, a block on one item does not leak into the next, and extra context
// entries reach the script without overriding the gateway's own.
func TestRunRequestFiltersWith_RunnersReusedAcrossBodies(t *testing.T) {
	p := newFilterTestProxy()
	llm := &models.LLM{ID: 42, Vendor: models.ANTHROPIC, Filters: []*models.Filter{
		{Name: "response-only", ResponseFilter: true, Script: []byte(`output := {block: true, message: "never on requests"}`)},
		{Name: "redact", Script: []byte(`
text := import("text")
msgs := []
for m in input.messages {
    msgs = append(msgs, {role: m.role, content: text.replace(m.content, "SECRET", "[REDACTED]", -1)})
}
output := {block: false, messages: msgs}
`)},
		{Name: "item-guard", Script: []byte(`
text := import("text")
last := input.messages[len(input.messages)-1]
output := {
    block: text.contains(last.content, "forbidden") || input.context.llm_id != 42,
    message: "item " + input.context.custom_id + " refused"
}
`)},
	}}
	runners := requestFilterRunners(llm)
	require.Len(t, runners, 2, "response filters are not request runners")

	run := func(customID, content string) (string, error) {
		body := `{"model":"claude-x","max_tokens":10,"messages":[{"role":"user","content":"` + content + `"}]}`
		r := httptest.NewRequest("POST", "/v1/messages/batches", strings.NewReader(body))
		out, err := p.runRequestFiltersWith(runners, llm, r, []byte(body), "anthropic", "claude-x",
			map[string]interface{}{"custom_id": customID, "llm_id": int64(7)})
		return string(out), err
	}

	out, err := run("a", "my SECRET plan")
	require.NoError(t, err)
	require.Contains(t, out, "[REDACTED]")
	require.NotContains(t, out, "SECRET plan")

	_, err = run("b", "something forbidden")
	require.Error(t, err)
	require.Contains(t, err.Error(), "item b refused")

	out, err = run("c", "plain")
	require.NoError(t, err, "a block on an earlier body must not carry over")
	require.Contains(t, out, "plain")

	// No runners is a pass-through, whatever the LLM holds.
	body := []byte(`{"messages":[{"role":"user","content":"forbidden"}]}`)
	got, err := p.runRequestFiltersWith(nil, llm, httptest.NewRequest("POST", "/", nil), body, "anthropic", "", nil)
	require.NoError(t, err)
	require.Equal(t, body, got)
}
