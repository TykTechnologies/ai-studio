package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// What the documentation says about signed resource IDs, pinned
// (midsommar-clean-6bg5.27). The signature is what lets a gateway that did
// not create a resource accept its ID. The gateway that created it also has
// the resource in its ledger, under the App that created it, and accepts the
// ID from that App whatever its signature says (the key-rotation fallback of
// unwrapID). So:
//
//   - another App never gets anything but 404, on any gateway;
//   - an ID with an altered signature, and an ID signed with a key that was
//     rotated out with no grace period, are 404 on every gateway except the
//     one that created the resource, where the owning App still gets 200.
func TestNativeLedgerFallback_DocumentedBehavior(t *testing.T) {
	t.Parallel()
	alter := func(id string) string {
		return id[:len(id)-1] + map[bool]string{true: "0", false: "1"}[strings.HasSuffix(id, "1")]
	}

	t.Run("the gateway that created the resource", func(t *testing.T) {
		t.Parallel()
		h := newNativeHarness(t, nativeHarnessOpts{})
		h.vendor.addBatch("msgbatch_L1", "ended")
		h.seedBatchRow(h.llm, h.appA, "msgbatch_L1") // created here, by App A
		good := h.wrap(h.llm, h.appA, "msgbatch_L1")

		status, _ := h.getBatch(h.keyA, alter(good))
		assert.Equal(t, http.StatusOK, status, "the owner's altered ID is accepted through the ledger")
		zeroMAC := good[:strings.LastIndex(good, ".")+1] + strings.Repeat("0", 32)
		status, _ = h.getBatch(h.keyA, zeroMAC)
		assert.Equal(t, http.StatusOK, status, "any signature, for the owner")

		// Another App: 404 for the valid ID and for every variation.
		for _, id := range []string{good, alter(good), zeroMAC, "msgbatch_L1"} {
			status, _ = h.getBatch(h.keyB, id)
			assert.Equal(t, http.StatusNotFound, status, "another App never reaches it: %s", id)
		}
		// A raw vendor ID is not a signed ID at all: 404 for the owner too.
		status, _ = h.getBatch(h.keyA, "msgbatch_L1")
		assert.Equal(t, http.StatusNotFound, status)

		// Rotation with no grace period: the old ID still works here for
		// its owner, and still not for anyone else.
		h.retryLocked(func() error { return models.RotateLLMResourceSigningSecret(h.db, h.llm.ID, 0, time.Now()) })
		h.reloadLLM()
		status, batch := h.getBatch(h.keyA, good)
		require.Equal(t, http.StatusOK, status, "the creating gateway keeps accepting the owner's old ID")
		assert.Equal(t, h.wrap(h.llm, h.appA, "msgbatch_L1"), batch["id"], "the answer carries the ID under the new key")
		status, _ = h.getBatch(h.keyB, good)
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("any other gateway", func(t *testing.T) {
		t.Parallel()
		h := newNativeHarness(t, nativeHarnessOpts{})
		h.vendor.addBatch("msgbatch_L2", "ended") // exists at the vendor; no ledger row here
		good := h.wrap(h.llm, h.appA, "msgbatch_L2")

		status, _ := h.getBatch(h.keyA, good)
		require.Equal(t, http.StatusOK, status, "a valid signature is what another gateway accepts")
		status, _ = h.getBatch(h.keyA, alter(good))
		assert.Equal(t, http.StatusNotFound, status, "an altered ID is refused")
		status, _ = h.getBatch(h.keyB, good)
		assert.Equal(t, http.StatusNotFound, status, "another App is refused")

		h.retryLocked(func() error { return models.RotateLLMResourceSigningSecret(h.db, h.llm.ID, 0, time.Now()) })
		h.reloadLLM()
		status, _ = h.getBatch(h.keyA, good)
		assert.Equal(t, http.StatusNotFound, status, "a rotation with no grace period revokes the old ID here at once")
	})
}
