package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Anything after the array is not a valid body: a producer that framed its
// output wrongly (NDJSON, concatenated arrays) must hear about it rather
// than lose the tail silently (midsommar-clean-05q).
func TestIngestTykAnalytics_TrailingData(t *testing.T) {
	h := setupIngestAPI(t)
	for name, body := range map[string]string{
		"two arrays":        `[{"api_id":"a"}][{"api_id":"b"}]`,
		"array then ndjson": "[{\"api_id\":\"a\"}]\n{\"api_id\":\"b\"}\n",
		"garbage":           `[] trailing`,
	} {
		w := h.ingest(t, "Bearer tai_ing_good", []byte(body), nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", name, w.Body.String())
	}
	assert.Empty(t, h.fake.batches)

	// whitespace after the array is fine
	w := h.ingest(t, "Bearer tai_ing_good", []byte("[]\n\n"), nil)
	assert.Equal(t, http.StatusAccepted, w.Code)
}
