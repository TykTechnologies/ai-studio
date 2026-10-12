//go:build enterprise

package proxy

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// One runner set shared by concurrent items (the batch create worker pool,
// ut0.5) must not mix items up: each item gets its own redaction, its own
// block decision and its own custom_id in the message. Added during
// independent verification (ut0.11); run with -race.
func TestRunRequestFiltersWith_ConcurrentItemsShareRunners(t *testing.T) {
	p := newFilterTestProxy()
	llm := &models.LLM{ID: 42, Vendor: models.ANTHROPIC, Filters: []*models.Filter{
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
    block: text.contains(last.content, "forbidden"),
    message: "item " + input.context.custom_id + " refused"
}
`)},
	}}
	runners := requestFilterRunners(llm)

	const n = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := fmt.Sprintf("item-%d", i)
			content := fmt.Sprintf("SECRET payload %d", i)
			blocked := i%3 == 0
			if blocked {
				content = fmt.Sprintf("forbidden payload %d", i)
			}
			body := `{"model":"claude-x","max_tokens":10,"messages":[{"role":"user","content":"` + content + `"}]}`
			r := httptest.NewRequest("POST", "/v1/messages/batches", strings.NewReader(body))
			out, err := p.runRequestFiltersWith(runners, llm, r, []byte(body), "anthropic", "claude-x",
				map[string]interface{}{"custom_id": id})
			if blocked {
				if assert.Error(t, err, id) {
					assert.Contains(t, err.Error(), "item "+id+" refused")
				}
				return
			}
			if assert.NoError(t, err, id) {
				assert.Contains(t, string(out), fmt.Sprintf("[REDACTED] payload %d", i), id)
				assert.NotContains(t, string(out), "SECRET", id)
			}
		}(i)
	}
	close(start)
	wg.Wait()
}
