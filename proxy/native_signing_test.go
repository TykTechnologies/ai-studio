package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// reloadLLM re-reads h.llm from the database and reloads the proxy, as an
// LLM edit does.
func (h *nativeHarness) reloadLLM() {
	h.t.Helper()
	var fresh models.LLM
	h.retryLocked(func() error { fresh = models.LLM{}; return h.db.First(&fresh, h.llm.ID).Error })
	h.llm = &fresh
	require.NoError(h.t, h.proxy.loadResources())
}

// setLLMColumn updates one column of h.llm. The shared-cache SQLite refuses
// a write that overlaps the analytics writer ("database table is locked");
// retry it.
func (h *nativeHarness) setLLMColumn(column string, value any) {
	h.t.Helper()
	h.retryLocked(func() error {
		return h.db.Model(&models.LLM{}).Where("id = ?", h.llm.ID).Update(column, value).Error
	})
}

// retryLocked runs a database call until the shared-cache table lock lets it
// through.
func (h *nativeHarness) retryLocked(fn func() error) {
	h.t.Helper()
	var err error
	for attempt := 0; attempt < 100; attempt++ {
		if err = fn(); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(h.t, err)
}

func (h *nativeHarness) getBatch(key, id string) (int, map[string]any) {
	h.t.Helper()
	resp, body := h.do(key, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, decodeBatch(h.t, body)
}

// IDs are signed with the hub-generated secret, so rotating the vendor
// credential leaves them valid on every node.
func TestNative_VendorKeyRotationKeepsIDs(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	require.NotEmpty(t, h.llm.ResourceSigningSecret, "a new LLM gets a signing secret")
	id := h.wrap(h.llm, h.appA, "msgbatch_A1")

	h.setLLMColumn("api_key", "rotated-vendor-key")
	h.reloadLLM()
	require.Equal(t, "rotated-vendor-key", h.llm.APIKey)

	status, batch := h.getBatch(h.keyA, id)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, id, batch["id"], "the ID is unchanged")
}

// Rotating the signing secret mints new IDs under a new kid and keeps the
// old ones verifiable for the grace period; a rotation without grace revokes
// them at once.
func TestNative_SigningSecretRotation(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	first := h.wrap(h.llm, h.appA, "msgbatch_A1")

	h.retryLocked(func() error { return models.RotateLLMResourceSigningSecret(h.db, h.llm.ID, time.Hour, time.Now()) })
	h.reloadLLM()
	second := h.wrap(h.llm, h.appA, "msgbatch_A1")
	require.NotEqual(t, first, second)

	status, batch := h.getBatch(h.keyA, first)
	require.Equal(t, http.StatusOK, status, "the previous key verifies in its grace period")
	assert.Equal(t, second, batch["id"], "answers carry IDs under the new key")

	// The grace scope is still the App's: B cannot use A's old ID.
	status, _ = h.getBatch(h.keyB, first)
	assert.Equal(t, http.StatusNotFound, status)

	h.retryLocked(func() error { return models.RotateLLMResourceSigningSecret(h.db, h.llm.ID, 0, time.Now()) })
	h.reloadLLM()
	for _, id := range []string{first, second} {
		status, _ = h.getBatch(h.keyA, id)
		assert.Equal(t, http.StatusNotFound, status, "a rotation without grace revokes the previous key")
	}
	status, _ = h.getBatch(h.keyA, h.wrap(h.llm, h.appA, "msgbatch_A1"))
	assert.Equal(t, http.StatusOK, status)
}

// An LLM created before signing secrets signs with its vendor-credential
// key. The upgrade backfill gives it a secret and keeps the IDs it already
// handed out valid until the grace period ends, even across a vendor key
// change.
func TestNative_BackfillKeepsVendorCredentialIDs(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) {
		l.ResourceSigningSecret, l.ResourceSigningRotatedAt = "", nil
	}})
	h.vendor.addBatch("msgbatch_A1", "ended")
	h.reloadLLM()
	require.Empty(t, h.llm.ResourceSigningSecret)
	legacy := wrapBatchID(t, h.llm.APIKey, h.llm.ID, h.appA.ID, "msgbatch_A1")
	require.Equal(t, legacy, h.wrap(h.llm, h.appA, "msgbatch_A1"), "no secret: the vendor-credential key signs")
	status, _ := h.getBatch(h.keyA, legacy)
	require.Equal(t, http.StatusOK, status)

	n, err := models.BackfillLLMResourceSigningSecrets(h.db, time.Hour, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n, "only the LLM without a secret is backfilled")
	h.reloadLLM()
	current := h.wrap(h.llm, h.appA, "msgbatch_A1")
	require.NotEqual(t, legacy, current)

	status, batch := h.getBatch(h.keyA, legacy)
	require.Equal(t, http.StatusOK, status, "vendor-credential IDs verify in the grace period")
	assert.Equal(t, current, batch["id"])

	// The backfill stored the upgrade-time key: changing the vendor
	// credential during the grace period keeps those IDs valid.
	h.setLLMColumn("api_key", "rotated-vendor-key")
	h.reloadLLM()
	status, _ = h.getBatch(h.keyA, legacy)
	require.Equal(t, http.StatusOK, status, "a vendor key change in the grace period keeps pre-upgrade IDs")
	status, _ = h.getBatch(h.keyB, wrapBatchID(t, "anthropic-vendor-key", h.llm.ID, h.appA.ID, "msgbatch_A1"))
	assert.Equal(t, http.StatusNotFound, status, "still scoped to the App")

	h.setLLMColumn("resource_signing_prev_expires_at", time.Now().Add(-time.Minute))
	h.reloadLLM()
	status, _ = h.getBatch(h.keyA, legacy)
	assert.Equal(t, http.StatusNotFound, status, "past the grace period the vendor-credential key is gone")
	status, _ = h.getBatch(h.keyA, current)
	assert.Equal(t, http.StatusOK, status)
}
