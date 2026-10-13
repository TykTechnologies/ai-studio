package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

func createBatch(h *responsesHarness, file string) (*http.Response, []byte) {
	h.t.Helper()
	return h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches",
		`{"input_file_id":"`+file+`","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
}

// write retries fn while the shared-cache SQLite refuses it ("database
// table is locked") for overlapping a background writer of the gateway.
func (h *responsesHarness) write(fn func() error) {
	h.t.Helper()
	var err error
	for attempt := 0; attempt < 100; attempt++ {
		if err = fn(); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(h.t, err)
}

func (h *responsesHarness) setFilters(filters ...*models.Filter) {
	h.t.Helper()
	h.write(func() error { return h.db.Model(h.primary).Association("Filters").Clear() })
	h.primary.Filters = filters
	h.write(func() error { return h.db.Save(h.primary).Error })
	require.NoError(h.t, h.proxy.loadResources())
}

// requireScreenedElsewhere asserts the create was refused before it reached
// the vendor, telling the client to upload again and naming no filter.
func requireScreenedElsewhere(t *testing.T, h *responsesHarness, resp *http.Response, out []byte, creates int) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	typ, msg := oaiErr(t, out)
	assert.Equal(t, nativeErrInvalidRequest, typ)
	assert.Contains(t, msg, "upload it again")
	assert.NotContains(t, msg, "remote guard")
	assert.Len(t, h.api.callsTo(http.MethodPost, "/v1/batches"), creates, "a refused create never reaches the vendor")
}

// A batch create runs its input file's lines, which were screened when the
// file was uploaded: it is refused unless the LLM still applies the request
// filters that screened them.
func TestOpenAIBatches_CreateNeedsTheFiltersThatScreenedTheFile(t *testing.T) {
	h := newBatchesHarness(t)
	ctx := context.Background()
	store := h.service.NativeResourceStore()

	// Uploaded while the LLM screened nothing.
	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	unscreened := decodeObj(t, out)["id"].(string)
	row, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceFile, vendorPart(unscreened))
	require.NoError(t, err)
	assert.Equal(t, nativeScreeningNone, row.ScreeningDigest)

	// A guardrail is attached: the old file no longer runs.
	h.setFilters(remoteGuardFilter())
	resp, out = createBatch(h, unscreened)
	requireScreenedElsewhere(t, h, resp, out, 0)

	// A file uploaded under the guardrail runs.
	resp, out = h.uploadFile(h.keyC, chatLine("r2", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	screened := decodeObj(t, out)["id"].(string)
	resp, out = createBatch(h, screened)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)

	// The guardrail is edited: files it screened before no longer run.
	filter := h.primary.Filters[0]
	cfg := models.JSONMap{}
	for k, v := range filter.Config {
		cfg[k] = v
	}
	cfg["quarantine"] = map[string]any{"placeholder": "[withheld]"}
	h.write(func() error {
		return h.db.Model(&models.Filter{}).Where("id = ?", filter.ID).Update("config", cfg).Error
	})
	require.NoError(t, h.proxy.loadResources())
	resp, out = createBatch(h, screened)
	requireScreenedElsewhere(t, h, resp, out, 1)

	// Without request filters there is no policy to hold the file to.
	h.setFilters()
	resp, out = createBatch(h, unscreened)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
}

// A file row without a digest (uploaded before the gateway recorded one)
// and a file that was never screened (a batch's output file) do not run on
// an LLM with request filters.
func TestOpenAIBatches_CreateRefusesUnrecordedScreening(t *testing.T) {
	h := newBatchesHarness(t)
	h.setFilters(remoteGuardFilter())
	ctx := context.Background()
	store := h.service.NativeResourceStore()

	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file := decodeObj(t, out)["id"].(string)
	h.write(func() error {
		return h.db.Model(&models.NativeResource{}).
			Where("resource_type = ? AND vendor_resource_id = ?", openaiVendor.ResourceFile, vendorPart(file)).
			Update("screening_digest", "").Error
	})
	resp, out = createBatch(h, file)
	requireScreenedElsewhere(t, h, resp, out, 0)

	// The output file of a batch built from a fresh upload.
	resp, out = h.uploadFile(h.keyC, chatLine("r2", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	resp, out = createBatch(h, decodeObj(t, out)["id"].(string))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	batch := decodeObj(t, out)["id"].(string)
	h.api.completeBatch(vendorPart(batch), outputLine("r2", 10, 5, 0))
	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/batches/"+batch, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	output := decodeObj(t, out)["output_file_id"].(string)
	_, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceFile, vendorPart(output))
	require.NoError(t, err, "the output file is in the ledger")
	resp, out = createBatch(h, output)
	requireScreenedElsewhere(t, h, resp, out, 1)
}

// A ledger that cannot be read is a 503, not a request to upload again.
func TestOpenAIBatches_CreateScreeningCheckLedgerDown(t *testing.T) {
	h := newBatchesHarness(t)
	h.setFilters(remoteGuardFilter())

	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file := decodeObj(t, out)["id"].(string)
	h.write(func() error { return h.db.Exec("DROP TABLE native_resources").Error })
	resp, out = createBatch(h, file)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "%s", out)
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/batches"))
}

func TestRequestFilterDigest(t *testing.T) {
	script := func(id uint, s string) *models.Filter {
		f := &models.Filter{Name: "f", Script: []byte(s)}
		f.ID = id
		return f
	}
	guard := func(id uint, placeholder string) *models.Filter {
		f := remoteGuardFilter()
		f.ID = id
		f.Config["quarantine"] = map[string]any{"placeholder": placeholder}
		return f
	}
	digest := func(fs ...*models.Filter) string {
		return requestFilterDigest(&models.LLM{Filters: fs})
	}
	response := script(9, "x")
	response.ResponseFilter = true

	assert.Equal(t, nativeScreeningNone, digest())
	assert.Equal(t, nativeScreeningNone, digest(response), "response filters screen no request")

	base := digest(script(1, "a"), guard(2, "p"))
	assert.Len(t, base, 64)
	assert.Equal(t, base, digest(script(1, "a"), guard(2, "p")), "stable")
	assert.Equal(t, base, digest(script(1, "a"), response, guard(2, "p")))

	scriptKind := script(1, "a")
	scriptKind.Kind = models.FilterKindScript
	assert.Equal(t, base, digest(scriptKind, guard(2, "p")), "an empty kind is a script")
	touched := script(1, "a")
	touched.UpdatedAt = time.Now()
	assert.Equal(t, base, digest(touched, guard(2, "p")), "a save that changes nothing")

	for name, other := range map[string]string{
		"order":       digest(guard(2, "p"), script(1, "a")),
		"script":      digest(script(1, "b"), guard(2, "p")),
		"config":      digest(script(1, "a"), guard(2, "q")),
		"filter id":   digest(script(3, "a"), guard(2, "p")),
		"one removed": digest(script(1, "a")),
		"boundary":    digest(script(1, "a|"), guard(2, "p")),
	} {
		assert.NotEqual(t, base, other, name)
	}

	// Connection values are not part of the policy: the hub holds a
	// $SECRET/ reference where an edge holds the resolved key, and a
	// rotated key screens the same way. Which fields are set still counts.
	connected := func(conn any) *models.Filter {
		f := guard(2, "p")
		f.Config["connection"] = conn
		return f
	}
	ref := digest(connected(map[string]any{"api_key": "$SECRET/GUARD_KEY", "endpoint": "https://guard.example"}))
	assert.Equal(t, ref, digest(connected(map[string]any{"api_key": "sk-resolved-1", "endpoint": "https://guard.example"})), "resolved on an edge")
	assert.Equal(t, ref, digest(connected(map[string]any{"api_key": "sk-resolved-2", "endpoint": "https://other.example"})), "rotated")
	assert.Equal(t, ref, digest(connected(map[string]string{"api_key": "x", "endpoint": "y"})))
	assert.NotEqual(t, ref, digest(connected(map[string]any{"api_key": "x"})), "a field dropped")
	assert.NotEqual(t, ref, digest(guard(2, "p")))
	withDetector := connected(map[string]any{"api_key": "x", "endpoint": "y"})
	withDetector.Config["detectors"] = []any{map[string]any{"name": "injection", "threshold": 0.9}}
	assert.NotEqual(t, ref, digest(withDetector), "a changed detector threshold")
	kept := connected(map[string]any{"api_key": "$SECRET/GUARD_KEY"})
	digest(kept)
	assert.Equal(t, "$SECRET/GUARD_KEY", kept.Config["connection"].(map[string]any)["api_key"], "the filter's own config is left alone")
}

// A guardrail's key rotated between upload and batch create (on an edge the
// config carries the resolved key) leaves the file usable; a policy change
// in the same filter does not.
func TestOpenAIBatches_CreateSurvivesKeyRotation(t *testing.T) {
	h := newBatchesHarness(t)
	guard := remoteGuardFilter()
	guard.Config["connection"] = map[string]any{"api_key": "sk-resolved-1"}
	h.setFilters(guard)

	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file := decodeObj(t, out)["id"].(string)

	cfg := models.JSONMap{}
	for k, v := range h.primary.Filters[0].Config {
		cfg[k] = v
	}
	cfg["connection"] = map[string]any{"api_key": "sk-resolved-2"}
	id := h.primary.Filters[0].ID
	h.write(func() error { return h.db.Model(&models.Filter{}).Where("id = ?", id).Update("config", cfg).Error })
	require.NoError(t, h.proxy.loadResources())
	resp, out = createBatch(h, file)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)

	resp, out = h.uploadFile(h.keyC, chatLine("r2", "gpt-4o", "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file = decodeObj(t, out)["id"].(string)
	cfg["on_detect"] = "log"
	h.write(func() error { return h.db.Model(&models.Filter{}).Where("id = ?", id).Update("config", cfg).Error })
	require.NoError(t, h.proxy.loadResources())
	resp, out = createBatch(h, file)
	requireScreenedElsewhere(t, h, resp, out, 1)
}
