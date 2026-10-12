//go:build enterprise

package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The request filters run on every line of a batch input file: a redaction
// rewrites only its line in the forwarded upload, a block refuses the upload
// naming the line's custom_id.
func TestOpenAIBatches_UploadFilters(t *testing.T) {
	h := newBatchesHarness(t)
	for i, s := range []string{nativeRedactScript, nativeBlockScript} {
		h.primary.Filters = append(h.primary.Filters, &models.Filter{Name: "oai-filter-" + string(rune('a'+i)), Script: []byte(s)})
	}
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	keep := chatLine("keep", "gpt-4o", "hello")
	resp, out := h.uploadFile(h.keyC, keep, chatLine("change", "gpt-4o", "my SECRET"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	uploads := h.api.callsTo(http.MethodPost, "/v1/files")
	require.Len(t, uploads, 1)
	sent := string(uploads[0].Body)
	assert.Contains(t, sent, keep, "an untouched line is forwarded byte for byte")
	assert.Contains(t, sent, "[REDACTED]")
	assert.NotContains(t, sent, "SECRET")
	h.api.mu.Lock()
	content := string(h.api.files["file-in1"].content)
	h.api.mu.Unlock()
	assert.Equal(t, 2, len(strings.Split(strings.TrimSpace(content), "\n")), "the vendor parsed the rebuilt upload: %q", content)

	resp, out = h.uploadFile(h.keyC, keep, chatLine("bad-one", "gpt-4o", "something forbidden"))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	_, msg := oaiErr(t, out)
	assert.Contains(t, msg, "no forbidden in bad-one")
	assert.Len(t, h.api.callsTo(http.MethodPost, "/v1/files"), 1)
}
