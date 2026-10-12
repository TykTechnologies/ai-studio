package proxy

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// Added during independent verification of the reconciler (ut0.18).

// Two reconcilers (replicas during a leadership flap, which the ledger CAS
// must survive without the gate) and a download all race on the same ended
// batch: one set of records, whatever the interleaving.
func TestNativeReconcilerVerify_ThreeWayRace(t *testing.T) {
	for name, open := range map[string]func(t *testing.T) *gorm.DB{
		"sqlite-file": func(t *testing.T) *gorm.DB {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")+"?_busy_timeout=10000&_journal_mode=WAL"),
				&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			t.Cleanup(func() {
				if s, err := db.DB(); err == nil {
					s.Close()
				}
			})
			require.NoError(t, db.AutoMigrate(&models.NativeResource{}))
			return db
		},
		"postgres": nativePostgresLedger,
	} {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			store := services.NewNativeResourceStore(db, db)
			h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
				return serviceWithNativeStore{ServiceInterface: s, store: store}
			}})
			h.addPrice("claude-luna", 0.00001, 0.000002)
			h.addPrice("claude-sol", 0.00002, 0.000004)
			ctx := context.Background()
			const rounds = 5
			for round := 0; round < rounds; round++ {
				vendorID := fmt.Sprintf("msgbatch_3WAY%d", round)
				h.vendor.addBatch(vendorID, "ended")
				h.vendor.setResults(vendorID, batchLines()...)
				row := &models.NativeResource{
					Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID, UserID: h.appA.UserID,
					ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: vendorID,
					State: "ended", AccountingState: models.NativeAccountingPending,
				}
				require.NoError(t, store.Create(ctx, row))

				var wg sync.WaitGroup
				start := make(chan struct{})
				accounted := make(chan nativeAccountOutcome, 2)
				for i := 0; i < 2; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						r := *row // each replica read its own copy of the row
						out, err := h.proxy.accountNativeRow(ctx, &r)
						assert.NoError(t, err)
						accounted <- out
					}()
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, vendorID)), "")
					assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
				}()
				close(start)
				wg.Wait()
				close(accounted)
				wins := 0
				for o := range accounted {
					if o == nativeAccountAccounted {
						wins++
					}
				}
				assert.LessOrEqual(t, wins, 1, "round %d: at most one reconciler records", round)
				got, err := store.GetByOwner(ctx, h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, vendorID)
				require.NoError(t, err)
				assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
			}
			count := func() int {
				h.proxy.waitForAnalyzers()
				var n int64
				if err := h.db.Model(&models.LLMChatRecord{}).Where("app_id = ?", h.appA.ID).Count(&n).Error; err != nil {
					return -1
				}
				return int(n)
			}
			require.Eventually(t, func() bool { return count() >= 2*rounds }, 10*time.Second, 50*time.Millisecond)
			h.settle()
			require.Eventually(t, func() bool { return count() >= 0 }, 5*time.Second, 50*time.Millisecond)
			assert.Equal(t, 2*rounds, count(), "two models per batch, each batch accounted once")
		})
	}
}
