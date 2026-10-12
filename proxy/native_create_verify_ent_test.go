//go:build enterprise

package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Added during independent verification of batch create (ut0.15).

// Go's encoding/json maps object keys onto struct fields case-insensitively,
// so the filter extractor reads "Messages" (or "System") as messages, while
// the vendor reads only the exact key. A params object carrying both
// spellings must not let the filters see one prompt and the vendor another.
func TestNativeCreateVerify_CaseVariantKeysDoNotBypassFilters(t *testing.T) {
	for name, params := range map[string]string{
		"Messages after messages": `{"model":"claude-luna","max_tokens":5,` +
			`"messages":[{"role":"user","content":"something forbidden"}],` +
			`"Messages":[{"role":"user","content":"hello"}]}`,
		"MESSAGES after messages": `{"model":"claude-luna","max_tokens":5,` +
			`"messages":[{"role":"user","content":"something forbidden"}],` +
			`"MESSAGES":[{"role":"user","content":"hello"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: withNativeFilters(nativeBlockScript)})
			body := `{"requests":[{"custom_id":"smuggle","params":` + params + `}]}`
			resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
			assert.NotEqual(t, http.StatusOK, resp.StatusCode, "filter bypassed: %s", out)
			assert.Empty(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/batches"),
				"a prompt the filters never saw reached the vendor")
		})
	}
}
