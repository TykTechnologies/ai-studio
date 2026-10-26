package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// Added during independent verification of list/delete (ut0.17).

// On Postgres (row locks, no harness table locks) concurrent deletes of an
// ended, unaccounted batch, racing the reconciler, must never reach the vendor
// before the results were read, succeed at most once, and leave exactly one
// set of records.
func TestNativeDeleteVerify_ConcurrentDeletesAndReconcilerOnPostgres(t *testing.T) {
	db := nativePostgresLedger(t)
	store := services.NewNativeResourceStore(db, db)
	h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
		return serviceWithNativeStore{ServiceInterface: s, store: store}
	}})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	ctx := context.Background()
	const rounds = 4
	for round := 0; round < rounds; round++ {
		vendorID := fmt.Sprintf("msgbatch_DEL%d", round)
		h.vendor.addBatch(vendorID, "ended")
		h.vendor.setResults(vendorID, succeededLine("a", "claude-luna", 100, 10))
		row := &models.NativeResource{
			Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID, UserID: h.appA.UserID,
			ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: vendorID,
			State: "ended", AccountingState: models.NativeAccountingPending,
		}
		require.NoError(t, store.Create(ctx, row))
		path := batchesPath + "/" + h.wrap(h.llm, h.appA, vendorID)

		var wg sync.WaitGroup
		start := make(chan struct{})
		codes := make(chan int, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				resp, _ := h.do(h.keyA, http.MethodDelete, path, "")
				codes <- resp.StatusCode
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := *row
			_, _ = h.proxy.accountNativeRow(ctx, &r)
		}()
		close(start)
		wg.Wait()
		close(codes)
		ok := 0
		for c := range codes {
			if c == http.StatusOK {
				ok++
			} else {
				assert.Contains(t, []int{http.StatusConflict, http.StatusNotFound}, c, "round %d", round)
			}
		}

		// Several deletes may reach the vendor once the row is settled (each
		// sees it accounted); the vendor answers all but the first 404, which
		// is harmless. What matters: one client success at most, and no
		// vendor delete before the results were read.
		assert.LessOrEqual(t, ok, 1, "round %d: at most one delete succeeds", round)
		calls := h.vendor.recorded()
		resultsAt, firstDelete := -1, -1
		for i, c := range calls {
			if c.Method == http.MethodGet && c.Path == "/v1/messages/batches/"+vendorID+"/results" && resultsAt < 0 {
				resultsAt = i
			}
			if c.Method == http.MethodDelete && c.Path == "/v1/messages/batches/"+vendorID && firstDelete < 0 {
				firstDelete = i
			}
		}
		if firstDelete >= 0 {
			assert.True(t, resultsAt >= 0 && resultsAt < firstDelete, "round %d: a vendor delete before the results were read", round)
		}
		got, err := store.GetByOwner(ctx, h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, vendorID)
		if ok == 1 {
			assert.ErrorIs(t, err, services.ErrNativeResourceNotFound, "round %d: deleted batch leaves the ledger", round)
		} else if err == nil {
			assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState, "round %d", round)
		}
	}
	count := func() int {
		h.proxy.waitForAnalyzers()
		var n int64
		if err := h.db.Model(&models.LLMChatRecord{}).Where("app_id = ?", h.appA.ID).Count(&n).Error; err != nil {
			return -1
		}
		return int(n)
	}
	require.Eventually(t, func() bool { return count() >= rounds }, 10*time.Second, 50*time.Millisecond)
	h.settle()
	require.Eventually(t, func() bool { return count() >= 0 }, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, rounds, count(), "each batch accounted exactly once")
}

// A listing never forwards to the vendor (which would list every App's
// batches in the workspace), even with vendor-style query parameters.
func TestNativeListVerify_NeverForwarded(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.seedBatchRow(h.llm, h.appA, "msgbatch_L1")
	h.vendor.addBatch("msgbatch_OTHER", "ended") // another App's batch at the vendor
	for _, q := range []string{"", "?limit=5", "?beta=true", "?limit=1&beta=true"} {
		resp, body := h.do(h.keyA, http.MethodGet, batchesPath+q, "")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.NotContains(t, string(body), "msgbatch_OTHER")
		assert.NotContains(t, string(body), `"msgbatch_L1"`, "listed IDs are wrapped")
	}
	assert.Empty(t, h.vendor.recorded(), "the vendor was never asked to list")
}
