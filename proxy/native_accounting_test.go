package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// claimSettleFlakyStore fails the first failClaims Claims and failSettles
// Settles with a transient store error.
type claimSettleFlakyStore struct {
	services.NativeResourceStore
	mu                      sync.Mutex
	failClaims, failSettles int
	claims, settles         int
}

func (s *claimSettleFlakyStore) Claim(ctx context.Context, id uint, token string, now time.Time, lease time.Duration) (bool, error) {
	s.mu.Lock()
	s.claims++
	fail := s.claims <= s.failClaims
	s.mu.Unlock()
	if fail {
		return false, fmt.Errorf("database is locked")
	}
	return s.NativeResourceStore.Claim(ctx, id, token, now, lease)
}

func (s *claimSettleFlakyStore) Settle(ctx context.Context, id uint, token, state string, now time.Time) (bool, error) {
	s.mu.Lock()
	s.settles++
	fail := s.settles <= s.failSettles
	s.mu.Unlock()
	if fail {
		return false, fmt.Errorf("database is locked")
	}
	return s.NativeResourceStore.Settle(ctx, id, token, state, now)
}

// A results download that meets transient store errors on its claim and
// settle retries them: the batch ends accounted by the download, recorded
// once. Errors that persist leave the row to the reconciler, unrecorded.
func TestNativeResults_DownloadAccountingRetriesStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		failClaims, failSettles int
		state                   string
		records                 int
	}{
		{"transient settle", 0, 2, models.NativeAccountingAccounted, 1},
		{"transient claim and settle", 2, 2, models.NativeAccountingAccounted, 1},
		{"persistent settle", 0, nativeSettleAttempts, models.NativeAccountingClaimed, 0},
		{"persistent claim", nativeSettleAttempts, 0, models.NativeAccountingPending, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &claimSettleFlakyStore{failClaims: tc.failClaims, failSettles: tc.failSettles}
			h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
				store.NativeResourceStore = s.(*services.Service).NativeResourceStore()
				return serviceWithNativeStore{ServiceInterface: s, store: store}
			}})
			h.addPrice("claude-luna", 0.00001, 0.000002)
			h.vendor.addBatch("msgbatch_A1", "ended")
			line := succeededLine("a", "claude-luna", 10, 5)
			h.vendor.setResults("msgbatch_A1", line)
			row := h.seedBatchRow(h.llm, h.appA, "msgbatch_A1")

			resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_A1")), "")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			assert.Equal(t, line, strings.TrimSpace(string(body)), "the client gets its results whatever the ledger does")

			got, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, row.ResourceType, "msgbatch_A1")
			require.NoError(t, err)
			assert.Equal(t, tc.state, got.AccountingState)
			if tc.records > 0 {
				require.Eventually(t, func() bool { return len(h.chatRecords(h.appA)) == tc.records }, 5*time.Second, 20*time.Millisecond)
			}
			time.Sleep(100 * time.Millisecond)
			assert.Len(t, h.chatRecords(h.appA), tc.records)
		})
	}
}
