package proxy

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// Adversarial cases added during independent verification of results
// downloads (ut0.16).

func (h *nativeHarness) batchRow(vendorID string) *models.NativeResource {
	h.t.Helper()
	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, vendorID)
	require.NoError(h.t, err)
	return row
}

// Many complete downloads at once still account the batch exactly once:
// one chat record per model, one spend per model.
func TestNativeResultsVerify_ConcurrentDownloadsAccountOnce(t *testing.T) {
	// The ledger is a WAL SQLite file with a busy timeout, as on an edge: the
	// harness's shared-cache :memory: database answers concurrent writers
	// "table is locked" at once, which tests the harness, not the CAS.
	ledger, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")+"?_busy_timeout=10000&_journal_mode=WAL"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if s, err := ledger.DB(); err == nil {
			s.Close()
		}
	})
	require.NoError(t, ledger.AutoMigrate(&models.NativeResource{}))
	store := services.NewNativeResourceStore(ledger, ledger)
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{
		wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend },
		wrapService: func(s services.ServiceInterface) services.ServiceInterface {
			return ledgerAndSpendService{serviceWithNativeStore{ServiceInterface: s, store: store}, spend}
		},
	})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.addPrice("claude-sol", 0.00002, 0.000004)
	h.vendor.addBatch("msgbatch_CC", "ended")
	h.vendor.setResults("msgbatch_CC",
		succeededLine("a", "claude-luna", 100, 10),
		succeededLine("b", "claude-sol", 100, 10),
		succeededLine("c", "claude-luna", 100, 10))
	require.NoError(t, store.Create(context.Background(), &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID, UserID: h.appA.UserID,
		ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: "msgbatch_CC",
		State: "ended", AccountingState: models.NativeAccountingPending,
	}))
	id := h.wrap(h.llm, h.appA, "msgbatch_CC")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, body := h.do(h.keyA, http.MethodGet, resultsPath(id), "")
			assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		}()
	}
	wg.Wait()

	// The harness DB is shared-cache SQLite: reads can hit "table is locked"
	// while the analytics writer runs, so poll instead of failing on one.
	countRecords := func() int {
		h.proxy.waitForAnalyzers()
		var n int64
		if err := h.db.Model(&models.LLMChatRecord{}).Where("app_id = ?", h.appA.ID).Count(&n).Error; err != nil {
			return -1
		}
		return int(n)
	}
	require.Eventually(t, func() bool { return countRecords() >= 2 }, 10*time.Second, 50*time.Millisecond)
	h.settle()
	require.Eventually(t, func() bool { return countRecords() >= 0 }, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, 2, countRecords(), "8 concurrent downloads, one record per model")
	_, calls := spend.totals()
	assert.Equal(t, 2, calls, "spend recorded once per model")
	got, err := store.GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_CC")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
}

// The vendor connection dropping mid-file is not a complete download: nothing
// is recorded and the row stays claimable.
func TestNativeResultsVerify_UpstreamTruncationRecordsNothing(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_TR", "ended")
	h.vendor.mu.Lock()
	h.vendor.resultsStream = func(w io.Writer) {
		_, _ = io.WriteString(w, succeededLine("a", "claude-luna", 100, 10)+"\n")
		if f, ok := w.(interface{ Flush() error }); ok { // gzip.Writer
			_ = f.Flush()
		}
		panic(http.ErrAbortHandler) // drop the connection mid-body
	}
	h.vendor.mu.Unlock()
	h.seedBatchRow(h.llm, h.appA, "msgbatch_TR")
	id := h.wrap(h.llm, h.appA, "msgbatch_TR")

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+resultsPath(id), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.keyA)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	assert.Empty(t, h.chatRecords(h.appA), "a truncated download must not account the batch")
	assert.Equal(t, models.NativeAccountingPending, h.batchRow("msgbatch_TR").AccountingState)
}

// A results line over the 16 MiB line cap ends the download incomplete:
// nothing recorded, the row stays claimable (for the reconciler).
func TestNativeResultsVerify_OversizedLineRecordsNothing(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_OL", "ended")
	huge := `{"custom_id":"big","result":{"type":"succeeded","message":{"model":"claude-luna","content":[{"type":"text","text":"` +
		strings.Repeat("x", 17<<20) + `"}],"usage":{"input_tokens":1,"output_tokens":1}}}}`
	h.vendor.setResults("msgbatch_OL", succeededLine("a", "claude-luna", 100, 10), huge, succeededLine("b", "claude-luna", 100, 10))
	h.seedBatchRow(h.llm, h.appA, "msgbatch_OL")
	id := h.wrap(h.llm, h.appA, "msgbatch_OL")

	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(id), "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Less(t, len(body), 1<<20, "the oversized line is not passed through")
	assert.Empty(t, h.chatRecords(h.appA))
	assert.Equal(t, models.NativeAccountingPending, h.batchRow("msgbatch_OL").AccountingState)
}
