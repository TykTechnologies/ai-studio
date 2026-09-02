//go:build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Issue #535: a script fault that panics inside the Tengo VM used to unwind
// out of RunScript and kill the API process. The filter editor's Test button
// made that a one-typo outage for an admin authoring a script, and because
// this handler runs the script on its own goroutine no caller-side recover
// could ever have caught it.
func TestFilterTestEndpoint_PanickingScriptReturnsError(t *testing.T) {
	api, _ := setupTestAPI(t)

	for _, script := range []string{
		`output := 5 / 0`,
		`output := 5 % 0`,
	} {
		// If the panic escaped, this request would take the test binary with it.
		w := performRequest(api.router, "POST", "/api/v1/filters/test", FilterTestInput{
			Script: script,
			Input: map[string]interface{}{
				"raw_input":   `{"messages":[{"role":"user","content":"hi"}]}`,
				"vendor_name": "openai",
				"model_name":  "gpt-4",
			},
		})

		assert.Equal(t, http.StatusOK, w.Code, "script %q", script)

		var response FilterTestOutput
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.False(t, response.Success, "script %q should report failure", script)
		assert.Contains(t, response.Error, "panic",
			"script %q should tell the author their script panicked", script)
	}
}
