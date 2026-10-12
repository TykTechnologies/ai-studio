package services

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adversarial cases added during independent verification of the ledger
// (ut0.13).

// A keyset cursor whose timestamp ties with other owners' rows must not leak
// them: the "created_at = ? AND id < ?" tie-breaker is an OR inside the owner
// filter, so it is only safe if it stays parenthesised.
func TestNativeResourceStoreVerify_KeysetTieNeverLeaksOtherOwners(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		tie := time.Now().Add(-time.Hour).Truncate(time.Second)

		mk := func(app, llm uint, typ, vid string) {
			r := newBatchRow(app, vid)
			r.LLMID, r.ResourceType, r.CreatedAt = llm, typ, tie
			require.NoError(t, st.Create(ctx, r))
		}
		const typ = "anthropic.message_batch"
		// Foreign rows on both sides of the App's rows in id order, all at
		// the same timestamp.
		mk(2, 7, typ, "foreign_app_lo")
		mk(1, 8, typ, "foreign_llm_lo")
		mk(1, 7, "openai.batch", "foreign_type_lo")
		mk(1, 7, typ, "own_a")
		mk(1, 7, typ, "own_b")
		mk(2, 7, typ, "foreign_app_hi")
		mk(1, 8, typ, "foreign_llm_hi")
		mk(1, 7, "openai.batch", "foreign_type_hi")

		ids := func(rows []models.NativeResource) []string {
			out := []string{}
			for _, r := range rows {
				out = append(out, r.VendorResourceID)
			}
			return out
		}
		q := NativeResourceListQuery{LLMID: 7, AppID: 1, ResourceType: typ, Limit: 100}

		all, err := st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"own_b", "own_a"}, ids(all))

		q.AfterVendorID = "own_b"
		page, err := st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"own_a"}, ids(page), "after-cursor tie must not pull in other owners' older ids")

		q.AfterVendorID, q.BeforeVendorID = "", "own_a"
		page, err = st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"own_b"}, ids(page), "before-cursor tie must not pull in other owners' newer ids")

		for _, foreign := range []string{"foreign_app_lo", "foreign_llm_hi", "foreign_type_lo"} {
			q.AfterVendorID, q.BeforeVendorID = foreign, ""
			_, err = st.List(ctx, q)
			assert.ErrorIs(t, err, ErrNativeResourceNotFound, "cursor %s belongs to another owner", foreign)
		}
	})
}

// Many reconcilers racing to take over the same expired claim: one winner,
// and the previous holder can neither settle nor release afterwards.
func TestNativeResourceStoreVerify_ConcurrentLeaseTakeover(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_takeover")
		require.NoError(t, st.Create(ctx, r))

		t0 := time.Now()
		ok, err := st.Claim(ctx, r.ID, "stale", t0, time.Minute)
		require.NoError(t, err)
		require.True(t, ok)

		later := t0.Add(5 * time.Minute)
		const n = 16
		var wins atomic.Int32
		var winner atomic.Value
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				tok := fmt.Sprintf("taker-%d", i)
				ok, err := st.Claim(ctx, r.ID, tok, later, time.Minute)
				assert.NoError(t, err)
				if ok {
					wins.Add(1)
					winner.Store(tok)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		require.Equal(t, int32(1), wins.Load(), "exactly one reconciler takes over an expired claim")

		ok, err = st.Settle(ctx, r.ID, "stale", models.NativeAccountingAccounted, later)
		require.NoError(t, err)
		assert.False(t, ok)
		ok, err = st.Release(ctx, r.ID, "stale")
		require.NoError(t, err)
		assert.False(t, ok)

		ok, err = st.Settle(ctx, r.ID, winner.Load().(string), models.NativeAccountingAccounted, later)
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

// The holder racing itself (a download finishing while a release fires, or a
// retried settle) must produce exactly one transition.
func TestNativeResourceStoreVerify_ConcurrentSettleAndReleaseOneEffect(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		for round := 0; round < 5; round++ {
			r := newBatchRow(1, fmt.Sprintf("msgbatch_self_%d", round))
			require.NoError(t, st.Create(ctx, r))
			now := time.Now()
			ok, err := st.Claim(ctx, r.ID, "holder", now, time.Minute)
			require.NoError(t, err)
			require.True(t, ok)

			var effects atomic.Int32
			var settled atomic.Int32
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					var ok bool
					var err error
					if i%2 == 0 {
						ok, err = st.Settle(ctx, r.ID, "holder", models.NativeAccountingAccounted, now)
						if ok {
							settled.Add(1)
						}
					} else {
						ok, err = st.Release(ctx, r.ID, "holder")
					}
					assert.NoError(t, err)
					if ok {
						effects.Add(1)
					}
				}(i)
			}
			close(start)
			wg.Wait()
			require.Equal(t, int32(1), effects.Load(), "round %d: one transition only", round)

			got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", r.VendorResourceID)
			require.NoError(t, err)
			if settled.Load() == 1 {
				assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
			} else {
				assert.Equal(t, models.NativeAccountingPending, got.AccountingState)
				assert.Empty(t, got.ClaimToken)
			}
		}
	})
}

// Lease expiry is a time comparison in SQL. On SQLite the times are text, so
// check sub-second boundaries, including a claim at a whole second (rendered
// without a fraction) against a fractional cutoff and vice versa.
func TestNativeResourceStoreVerify_LeaseBoundaryPrecision(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		lease := time.Second
		for i, claimedAt := range []time.Time{
			time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),           // whole second
			time.Date(2026, 10, 12, 12, 0, 0, 500_000_000, time.UTC), // .5
			time.Date(2026, 10, 12, 12, 0, 0, 123_456_000, time.UTC), // µs
			time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("NZDT", 13*3600)),
		} {
			r := newBatchRow(1, fmt.Sprintf("msgbatch_prec_%d", i))
			require.NoError(t, st.Create(ctx, r))
			ok, err := st.Claim(ctx, r.ID, "a", claimedAt, lease)
			require.NoError(t, err)
			require.True(t, ok)

			ok, err = st.Claim(ctx, r.ID, "b", claimedAt.Add(lease-time.Millisecond), lease)
			require.NoError(t, err)
			assert.False(t, ok, "case %d: 1ms before expiry the claim is live", i)
			ok, err = st.Claim(ctx, r.ID, "b", claimedAt.Add(lease), lease)
			require.NoError(t, err)
			assert.False(t, ok, "case %d: at exactly the lease the claim is still live (strict <)", i)
			ok, err = st.Claim(ctx, r.ID, "b", claimedAt.Add(lease+time.Millisecond), lease)
			require.NoError(t, err)
			assert.True(t, ok, "case %d: 1ms after expiry the claim can be taken over", i)
		}
	})
}
