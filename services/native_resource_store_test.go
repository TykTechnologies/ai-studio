package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeStoreDBs runs fn on a SQLite file database and, when DATABASE_URL is
// set, on Postgres in a throwaway schema: the claims are compare-and-set
// updates whose concurrency behaviour differs between the two.
func nativeStoreDBs(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "native.db")+"?_busy_timeout=10000&_journal_mode=WAL"),
			&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&models.NativeResource{}))
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.Close()
			}
		})
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Skip("DATABASE_URL not set - skipping PostgreSQL tests")
		}
		admin, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		schema := fmt.Sprintf("native_store_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
		t.Cleanup(func() {
			admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			if sqlDB, err := admin.DB(); err == nil {
				sqlDB.Close()
			}
		})
		sep := "?"
		if strings.Contains(dbURL, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(dbURL+sep+"search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.Close()
			}
		})
		require.NoError(t, db.AutoMigrate(&models.NativeResource{}))
		fn(t, db)
	})
}

func newBatchRow(app uint, vendorID string) *models.NativeResource {
	return &models.NativeResource{
		Vendor:           string(models.ANTHROPIC),
		LLMID:            7,
		AppID:            app,
		UserID:           3,
		OnBehalfOf:       "alice@example.com",
		ResourceType:     "anthropic.message_batch",
		VendorResourceID: vendorID,
		State:            "in_progress",
		AccountingState:  models.NativeAccountingPending,
	}
}

func TestNativeResourceStore_CreateAndOwnership(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)

		r := newBatchRow(1, "msgbatch_01")
		require.NoError(t, st.Create(ctx, r))
		require.NotZero(t, r.ID)

		err := st.Create(ctx, newBatchRow(2, "msgbatch_01"))
		assert.ErrorIs(t, err, ErrNativeResourceExists, "the vendor ID is unique per LLM and type, whatever the App")

		other := newBatchRow(1, "msgbatch_01")
		other.LLMID = 8
		require.NoError(t, st.Create(ctx, other), "the same vendor ID on another LLM is another resource")

		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_01")
		require.NoError(t, err)
		assert.Equal(t, r.ID, got.ID)
		assert.Equal(t, "alice@example.com", got.OnBehalfOf)
		assert.Equal(t, models.NativeAccountingPending, got.AccountingState)

		_, err = st.GetByOwner(ctx, 7, 2, "anthropic.message_batch", "msgbatch_01")
		assert.ErrorIs(t, err, ErrNativeResourceNotFound, "another App never sees the row")
		_, err = st.GetByOwner(ctx, 9, 1, "anthropic.message_batch", "msgbatch_01")
		assert.ErrorIs(t, err, ErrNativeResourceNotFound)
		_, err = st.GetByOwner(ctx, 7, 1, "openai.batch", "msgbatch_01")
		assert.ErrorIs(t, err, ErrNativeResourceNotFound)

		noState := newBatchRow(1, "msgbatch_02")
		noState.AccountingState = ""
		assert.Error(t, st.Create(ctx, noState), "the accounting state must be explicit")
	})
}

func TestNativeResourceStore_DeleteThenRecreate(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)

		r := newBatchRow(1, "msgbatch_del")
		require.NoError(t, st.Create(ctx, r))
		require.NoError(t, st.Delete(ctx, r.ID))
		assert.ErrorIs(t, st.Delete(ctx, r.ID), ErrNativeResourceNotFound)
		_, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_del")
		assert.ErrorIs(t, err, ErrNativeResourceNotFound)

		require.NoError(t, st.Create(ctx, newBatchRow(1, "msgbatch_del")), "no tombstone blocks re-creating the vendor ID")
	})
}

func TestNativeResourceStore_ConcurrentClaimHasOneWinner(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_race")
		require.NoError(t, st.Create(ctx, r))

		now := time.Now()
		const n = 16
		var wins atomic.Int32
		var winner atomic.Value
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				token := fmt.Sprintf("tok-%d", i)
				ok, err := st.Claim(ctx, r.ID, token, now, time.Minute)
				if err != nil {
					errs <- err
					return
				}
				if ok {
					wins.Add(1)
					winner.Store(token)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.Equal(t, int32(1), wins.Load(), "exactly one settler wins the claim")

		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_race")
		require.NoError(t, err)
		assert.Equal(t, models.NativeAccountingClaimed, got.AccountingState)
		assert.Equal(t, winner.Load(), got.ClaimToken)
		require.NotNil(t, got.ClaimedAt)
	})
}

func TestNativeResourceStore_ClaimLeaseAndSettle(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_lease")
		require.NoError(t, st.Create(ctx, r))

		t0 := time.Now()
		lease := time.Minute
		ok, err := st.Claim(ctx, r.ID, "first", t0, lease)
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = st.Claim(ctx, r.ID, "second", t0.Add(30*time.Second), lease)
		require.NoError(t, err)
		assert.False(t, ok, "a live claim is not taken over")

		ok, err = st.Claim(ctx, r.ID, "second", t0.Add(2*time.Minute), lease)
		require.NoError(t, err)
		assert.True(t, ok, "an expired claim can be taken over")

		ok, err = st.Settle(ctx, r.ID, "first", models.NativeAccountingAccounted, t0.Add(3*time.Minute))
		require.NoError(t, err)
		assert.False(t, ok, "the settler that lost its claim cannot settle")
		ok, err = st.Release(ctx, r.ID, "first")
		require.NoError(t, err)
		assert.False(t, ok, "nor release it")

		_, err = st.Settle(ctx, r.ID, "second", models.NativeAccountingPending, t0)
		assert.Error(t, err, "pending is not a settled state")

		ok, err = st.Settle(ctx, r.ID, "second", models.NativeAccountingAccounted, t0.Add(3*time.Minute))
		require.NoError(t, err)
		assert.True(t, ok)

		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_lease")
		require.NoError(t, err)
		assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
		require.NotNil(t, got.AccountedAt)

		for _, at := range []time.Time{t0, t0.Add(time.Hour), t0.Add(365 * 24 * time.Hour)} {
			ok, err = st.Claim(ctx, r.ID, "late", at, lease)
			require.NoError(t, err)
			assert.False(t, ok, "an accounted row is never claimable again")
		}
		ok, err = st.Settle(ctx, r.ID, "second", models.NativeAccountingAccounted, t0.Add(time.Hour))
		require.NoError(t, err)
		assert.False(t, ok, "settling twice is a no-op")
	})
}

func TestNativeResourceStore_ReleaseReturnsToPending(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_release")
		require.NoError(t, st.Create(ctx, r))

		now := time.Now()
		ok, err := st.Claim(ctx, r.ID, "a", now, time.Minute)
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = st.Release(ctx, r.ID, "a")
		require.NoError(t, err)
		require.True(t, ok)

		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_release")
		require.NoError(t, err)
		assert.Equal(t, models.NativeAccountingPending, got.AccountingState)
		assert.Empty(t, got.ClaimToken)

		ok, err = st.Claim(ctx, r.ID, "b", now, time.Minute)
		require.NoError(t, err)
		assert.True(t, ok, "a released row is claimable at once")
	})
}

func TestNativeResourceStore_UnaccountableAndLostAreFinal(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		now := time.Now()
		for _, state := range []string{models.NativeAccountingUnaccountable, models.NativeAccountingLost} {
			r := newBatchRow(1, "msgbatch_"+state)
			require.NoError(t, st.Create(ctx, r))
			ok, err := st.Claim(ctx, r.ID, "t", now, time.Minute)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = st.Settle(ctx, r.ID, "t", state, now)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = st.Claim(ctx, r.ID, "again", now.Add(time.Hour), time.Minute)
			require.NoError(t, err)
			assert.False(t, ok, state+" is final")
		}
		none := newBatchRow(1, "file_none")
		none.AccountingState = models.NativeAccountingNone
		require.NoError(t, st.Create(ctx, none))
		ok, err := st.Claim(ctx, none.ID, "t", now, time.Minute)
		require.NoError(t, err)
		assert.False(t, ok, "a row with nothing to account is never claimed")
	})
}

func TestNativeResourceStore_DueAndPolling(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		now := time.Now()
		lease := time.Minute

		fresh := newBatchRow(1, "b_fresh") // never polled: due
		require.NoError(t, st.Create(ctx, fresh))
		later := newBatchRow(1, "b_later")
		require.NoError(t, st.Create(ctx, later))
		require.NoError(t, st.SchedulePoll(ctx, later.ID, now.Add(time.Hour), 2))
		past := newBatchRow(1, "b_past")
		require.NoError(t, st.Create(ctx, past))
		require.NoError(t, st.SchedulePoll(ctx, past.ID, now.Add(-time.Second), 0))
		live := newBatchRow(1, "b_live")
		require.NoError(t, st.Create(ctx, live))
		ok, err := st.Claim(ctx, live.ID, "x", now, lease)
		require.NoError(t, err)
		require.True(t, ok)
		stale := newBatchRow(1, "b_stale")
		require.NoError(t, st.Create(ctx, stale))
		ok, err = st.Claim(ctx, stale.ID, "y", now.Add(-2*lease), lease)
		require.NoError(t, err)
		require.True(t, ok)
		done := newBatchRow(1, "b_done")
		require.NoError(t, st.Create(ctx, done))
		ok, err = st.Claim(ctx, done.ID, "z", now, lease)
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = st.Settle(ctx, done.ID, "z", models.NativeAccountingAccounted, now)
		require.NoError(t, err)
		require.True(t, ok)

		due, err := st.Due(ctx, now, lease, 100)
		require.NoError(t, err)
		var ids []uint
		for _, r := range due {
			ids = append(ids, r.ID)
		}
		assert.ElementsMatch(t, []uint{fresh.ID, past.ID, stale.ID}, ids)

		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "b_later")
		require.NoError(t, err)
		assert.Equal(t, 2, got.PollFailures)

		due, err = st.Due(ctx, now, lease, 1)
		require.NoError(t, err)
		assert.Len(t, due, 1)

		assert.ErrorIs(t, st.SchedulePoll(ctx, 99999, now, 0), ErrNativeResourceNotFound)
	})
}

func TestNativeResourceStore_UpdateSnapshot(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_snap")
		require.NoError(t, st.Create(ctx, r))

		ended := time.Now().Add(-time.Minute)
		expires := ended.Add(29 * 24 * time.Hour)
		require.NoError(t, st.UpdateSnapshot(ctx, r.ID, NativeResourceSnapshot{
			State: "ended", Snapshot: `{"id":"msgbatch_snap.t1kx.y","processing_status":"ended"}`,
			EndedAt: &ended, ResultsExpireAt: &expires,
		}))
		got, err := st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "msgbatch_snap")
		require.NoError(t, err)
		assert.Equal(t, "ended", got.State)
		assert.JSONEq(t, `{"id":"msgbatch_snap.t1kx.y","processing_status":"ended"}`, got.Snapshot)
		require.NotNil(t, got.EndedAt)
		assert.WithinDuration(t, ended, *got.EndedAt, time.Millisecond)
		require.NotNil(t, got.ResultsExpireAt)
		assert.WithinDuration(t, expires, *got.ResultsExpireAt, time.Millisecond)
		assert.Equal(t, models.NativeAccountingPending, got.AccountingState, "a snapshot never touches accounting")

		assert.ErrorIs(t, st.UpdateSnapshot(ctx, 99999, NativeResourceSnapshot{}), ErrNativeResourceNotFound)
	})
}

func TestNativeResourceStore_ListKeyset(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		base := time.Now().Add(-time.Hour)
		// b0 oldest … b4 newest; b2 and b3 share a timestamp (ties break on id).
		at := []time.Duration{0, time.Minute, 2 * time.Minute, 2 * time.Minute, 3 * time.Minute}
		for i, d := range at {
			r := newBatchRow(1, fmt.Sprintf("b%d", i))
			r.CreatedAt = base.Add(d)
			require.NoError(t, st.Create(ctx, r))
		}
		// Rows of another App, another LLM and another type stay out.
		require.NoError(t, st.Create(ctx, newBatchRow(2, "other_app")))
		otherLLM := newBatchRow(1, "other_llm")
		otherLLM.LLMID = 8
		require.NoError(t, st.Create(ctx, otherLLM))
		otherType := newBatchRow(1, "other_type")
		otherType.ResourceType = "openai.batch"
		require.NoError(t, st.Create(ctx, otherType))

		vendorIDs := func(rows []models.NativeResource) []string {
			out := []string{}
			for _, r := range rows {
				out = append(out, r.VendorResourceID)
			}
			return out
		}
		q := NativeResourceListQuery{LLMID: 7, AppID: 1, ResourceType: "anthropic.message_batch", Limit: 2}

		page, err := st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"b4", "b3"}, vendorIDs(page))

		q.AfterVendorID = "b3"
		page, err = st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"b2", "b1"}, vendorIDs(page))

		q.AfterVendorID = "b1"
		page, err = st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"b0"}, vendorIDs(page))

		q.AfterVendorID = ""
		q.BeforeVendorID = "b1"
		page, err = st.List(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, []string{"b3", "b2"}, vendorIDs(page), "the page just newer than the cursor, newest first")

		q.BeforeVendorID = "b4"
		page, err = st.List(ctx, q)
		require.NoError(t, err)
		assert.Empty(t, page)

		q.BeforeVendorID = "other_app"
		_, err = st.List(ctx, q)
		assert.ErrorIs(t, err, ErrNativeResourceNotFound, "a cursor of another App is not found")

		q.AfterVendorID, q.BeforeVendorID = "b1", "b3"
		_, err = st.List(ctx, q)
		assert.Error(t, err)
	})
}

func TestNativeResourceStore_SweepExpired(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		now := time.Now()
		expired := now.Add(-time.Hour)
		future := now.Add(time.Hour)

		settled := newBatchRow(1, "settled_expired")
		settled.ResultsExpireAt = &expired
		require.NoError(t, st.Create(ctx, settled))
		ok, err := st.Claim(ctx, settled.ID, "t", now, time.Minute)
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = st.Settle(ctx, settled.ID, "t", models.NativeAccountingAccounted, now)
		require.NoError(t, err)
		require.True(t, ok)

		pending := newBatchRow(1, "pending_expired")
		pending.ResultsExpireAt = &expired
		require.NoError(t, st.Create(ctx, pending))

		current := newBatchRow(1, "settled_current")
		current.AccountingState = models.NativeAccountingNone
		current.ResultsExpireAt = &future
		require.NoError(t, st.Create(ctx, current))

		n, err := st.SweepExpired(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)

		_, err = st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "settled_expired")
		assert.ErrorIs(t, err, ErrNativeResourceNotFound)
		_, err = st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "pending_expired")
		assert.NoError(t, err, "an unsettled row stays for the reconciler to mark unaccountable")
		_, err = st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "settled_current")
		assert.NoError(t, err)
	})
}

func TestNativeResourceStore_ReadsAndWritesUseTheirHandles(t *testing.T) {
	dir := t.TempDir()
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "rw.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		return db
	}
	read, write := open(), open()
	require.NoError(t, read.AutoMigrate(&models.NativeResource{}))

	var writes atomic.Int32
	require.NoError(t, write.Callback().Create().Before("gorm:create").Register("count_writes", func(*gorm.DB) { writes.Add(1) }))
	require.NoError(t, write.Callback().Update().Before("gorm:update").Register("count_updates", func(*gorm.DB) { writes.Add(1) }))
	require.NoError(t, read.Callback().Create().Before("gorm:create").Register("no_writes", func(db *gorm.DB) { t.Error("create went to the read handle") }))
	require.NoError(t, read.Callback().Update().Before("gorm:update").Register("no_updates", func(db *gorm.DB) { t.Error("update went to the read handle") }))

	ctx := context.Background()
	st := NewNativeResourceStore(read, write)
	r := newBatchRow(1, "rw")
	require.NoError(t, st.Create(ctx, r))
	ok, err := st.Claim(ctx, r.ID, "t", time.Now(), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = st.GetByOwner(ctx, 7, 1, "anthropic.message_batch", "rw")
	require.NoError(t, err)
	assert.Equal(t, int32(2), writes.Load())

	assert.Nil(t, NewNativeResourceStore(nil, nil))
	var s *Service
	assert.Nil(t, s.NativeResourceStore())
	assert.Nil(t, (&Service{}).NativeResourceStore())
	assert.NotNil(t, (&Service{DB: read}).NativeResourceStore())
}

// Claiming again with the token that holds the claim succeeds (a retry after
// an ambiguous error); another token still loses.
func TestNativeResourceStore_ClaimIsIdempotentForItsToken(t *testing.T) {
	nativeStoreDBs(t, func(t *testing.T, db *gorm.DB) {
		ctx := context.Background()
		st := NewNativeResourceStore(db, db)
		r := newBatchRow(1, "msgbatch_idem")
		require.NoError(t, st.Create(ctx, r))
		now := time.Now()

		ok, err := st.Claim(ctx, r.ID, "mine", now, time.Minute)
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = st.Claim(ctx, r.ID, "mine", now.Add(time.Second), time.Minute)
		require.NoError(t, err)
		assert.True(t, ok, "the holder's retry succeeds")
		ok, err = st.Claim(ctx, r.ID, "theirs", now.Add(2*time.Second), time.Minute)
		require.NoError(t, err)
		assert.False(t, ok, "another token still loses a live claim")

		ok, err = st.Settle(ctx, r.ID, "mine", models.NativeAccountingAccounted, now)
		require.NoError(t, err)
		require.True(t, ok)
		ok, err = st.Claim(ctx, r.ID, "mine", now.Add(time.Hour), time.Minute)
		require.NoError(t, err)
		assert.False(t, ok, "a settled row is never claimed, not even by its last holder")
	})
}
