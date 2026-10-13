package proxy

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// fastNativeWorkers shortens the reconciler's timings for a test.
func fastNativeWorkers(t *testing.T) {
	t.Helper()
	interval, poll, maxBackoff := nativeReconcileInterval, nativePollInterval, nativePollMaxBackoff
	nativeReconcileInterval, nativePollInterval, nativePollMaxBackoff = 20*time.Millisecond, time.Hour, 4*time.Hour
	t.Cleanup(func() {
		nativeReconcileInterval, nativePollInterval, nativePollMaxBackoff = interval, poll, maxBackoff
	})
}

func (h *nativeHarness) ledgerRow(id uint) *models.NativeResource {
	h.t.Helper()
	var row models.NativeResource
	require.NoError(h.t, h.db.First(&row, id).Error)
	return &row
}

func batchLines() []string {
	return []string{
		succeededLine("a", "claude-luna", 1000, 200),
		erroredLine,
		succeededLine("b", "claude-luna", 500, 100),
		canceledLine,
		succeededLine("c", "claude-sol", 100, 10),
		expiredLine,
	}
}

// A batch that ends and is never downloaded is accounted by the reconciler:
// one chat record per model at half price, the budget charged, the identity
// of the creating request stamped, the ledger row settled - and a second pass
// (or a later download) records nothing more. Its calls carry the vendor key
// and API version and nothing of a client.
func TestNativeReconciler_AccountsUndownloadedBatchOnce(t *testing.T) {
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend }})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.addPrice("claude-sol", 0.00002, 0.000004)
	h.vendor.addBatch("msgbatch_R1", "ended")
	h.vendor.setResults("msgbatch_R1", batchLines()...)
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_R1")

	out, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountAccounted, out)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool { recs = h.chatRecords(h.appA); return len(recs) == 2 }, 5*time.Second, 20*time.Millisecond)
	byModel := map[string]models.LLMChatRecord{}
	for _, r := range recs {
		byModel[r.Name] = r
	}
	luna, sol := byModel["claude-luna"], byModel["claude-sol"]
	assert.Equal(t, 1500, luna.PromptTokens)
	assert.Equal(t, 300, luna.ResponseTokens)
	assert.InDelta(t, (300*0.00001+1500*0.000002)*0.5*10000, luna.Cost, 1e-9)
	assert.InDelta(t, (10*0.00002+100*0.000004)*0.5*10000, sol.Cost, 1e-9)
	assert.Equal(t, "user:alice", luna.OnBehalfOf)
	assert.Equal(t, "agent:bot", luna.ActingAgent)
	total, calls := spend.totals()
	assert.Equal(t, 2, calls)
	assert.InDelta(t, (luna.Cost+sol.Cost)/10000, total, 1e-12)

	got := h.ledgerRow(row.ID)
	assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
	assert.Equal(t, "ended", got.State)
	require.NotNil(t, got.EndedAt)
	require.NotNil(t, got.ResultsExpireAt, "the poll stores the results expiry so the row is swept later")
	assert.Contains(t, got.Snapshot, h.wrap(h.llm, h.appA, "msgbatch_R1"), "snapshot holds the wrapped object")
	assert.NotContains(t, got.Snapshot, "api.anthropic.com", "snapshot results_url points at the gateway")

	for _, c := range h.vendor.recorded() {
		assert.Equal(t, "anthropic-vendor-key", c.Headers.Get("x-api-key"), c.Path)
		assert.Equal(t, anthropicVendor.NativeAPIVersion, c.Headers.Get("anthropic-version"), c.Path)
		assert.Empty(t, c.Headers.Get("Authorization"), c.Path)
	}
	assert.Len(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_R1"), 1)
	assert.Len(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_R1/results"), 1)

	// Settled: neither another reconciliation nor a download records again.
	out, err = h.proxy.accountNativeRow(context.Background(), got)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountSettled, out)
	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_R1")), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Len(t, h.chatRecords(h.appA), 2)
	_, calls = spend.totals()
	assert.Equal(t, 2, calls)
}

// An unfinished batch is left pending with its snapshot refreshed; the
// reconciler schedules the next poll.
func TestNativeReconciler_NotEndedSchedulesNextPoll(t *testing.T) {
	fastNativeWorkers(t)
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_P1", "in_progress")
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_P1")

	before := time.Now()
	h.proxy.reconcileNativeRow(context.Background(), h.service.NativeResourceStore(), row, before)

	got := h.ledgerRow(row.ID)
	assert.Equal(t, models.NativeAccountingPending, got.AccountingState)
	assert.Equal(t, "in_progress", got.State)
	require.NotNil(t, got.NextPollAt)
	assert.WithinDuration(t, before.Add(nativePollInterval), *got.NextPollAt, time.Second)
	assert.Zero(t, got.PollFailures)
	assert.Empty(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_P1/results"))
	assert.Empty(t, h.chatRecords(h.appA))
}

// Results that expired before anyone read them settle the row unaccountable
// without calling the vendor; a batch the vendor no longer knows settles
// lost. Neither is due again.
func TestNativeReconciler_ExpiredAndLostAreTerminal(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	store := h.service.NativeResourceStore()
	ctx := context.Background()

	expired := h.seedBatchRow(h.llm, h.appA, "msgbatch_X1")
	past := time.Now().Add(-time.Hour)
	require.NoError(t, store.UpdateSnapshot(ctx, expired.ID, services.NativeResourceSnapshot{State: "ended", ResultsExpireAt: &past}))
	expired = h.ledgerRow(expired.ID)
	out, err := h.proxy.accountNativeRow(ctx, expired)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountUnaccountable, out)
	assert.Equal(t, models.NativeAccountingUnaccountable, h.ledgerRow(expired.ID).AccountingState)
	assert.Empty(t, h.vendor.recorded(), "an expired row is settled without a vendor call")

	lost := h.seedBatchRow(h.llm, h.appA, "msgbatch_GONE") // never added to the vendor
	out, err = h.proxy.accountNativeRow(ctx, lost)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountLost, out)
	assert.Equal(t, models.NativeAccountingLost, h.ledgerRow(lost.ID).AccountingState)

	due, err := store.Due(ctx, time.Now().Add(24*time.Hour), nativeClaimLease, 100)
	require.NoError(t, err)
	assert.Empty(t, due, "terminal rows are never polled again")
	assert.Empty(t, h.chatRecords(h.appA))

	// The sweep removes settled rows whose results expired.
	h.proxy.reconcileNative(ctx, store, time.Now())
	var n int64
	h.db.Model(&models.NativeResource{}).Where("id = ?", expired.ID).Count(&n)
	assert.Zero(t, n, "expired settled row swept")
}

// Results the vendor no longer serves for an ended batch settle it
// unaccountable.
func TestNativeReconciler_ResultsGoneIsUnaccountable(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_G1", "ended")
	h.vendor.resultsStatus = http.StatusNotFound
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_G1")
	out, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountUnaccountable, out)
	assert.Equal(t, models.NativeAccountingUnaccountable, h.ledgerRow(row.ID).AccountingState)
}

// A vendor error is transient: the row stays pending, its claim released,
// and is polled again with growing backoff.
func TestNativeReconciler_TransientErrorBacksOff(t *testing.T) {
	fastNativeWorkers(t)
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_E1", "ended")
	h.vendor.resultsStatus = http.StatusInternalServerError
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_E1")
	store := h.service.NativeResourceStore()

	now := time.Now()
	h.proxy.reconcileNativeRow(context.Background(), store, row, now)
	got := h.ledgerRow(row.ID)
	assert.Equal(t, models.NativeAccountingPending, got.AccountingState, "claim released")
	assert.Empty(t, got.ClaimToken)
	assert.Equal(t, 1, got.PollFailures)
	require.NotNil(t, got.NextPollAt)
	assert.WithinDuration(t, now.Add(nativePollInterval), *got.NextPollAt, time.Second)

	h.proxy.reconcileNativeRow(context.Background(), store, got, now)
	got = h.ledgerRow(row.ID)
	assert.Equal(t, 2, got.PollFailures)
	assert.WithinDuration(t, now.Add(2*nativePollInterval), *got.NextPollAt, time.Second)
	assert.Empty(t, h.chatRecords(h.appA))

	assert.Equal(t, nativePollMaxBackoff, (*Config)(nil).nativePollBackoff(50))
}

// A row whose LLM is gone stays pending (with backoff) until its results
// expire, then settles unaccountable.
func TestNativeReconciler_UnresolvableLLM(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_U1")
	row.LLMID = 999999
	_, err := h.proxy.accountNativeRow(context.Background(), row)
	assert.ErrorIs(t, err, errNativeRowUnresolvable)

	past := time.Now().Add(-time.Minute)
	row.ResultsExpireAt = &past
	out, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountUnaccountable, out)
}

// A results download and the reconciler racing on one batch record exactly
// one set of records, whichever settles first.
//
// The ledger is a SQLite file database like an edge's: the harness's
// shared-cache in-memory database answers concurrent writers with "table is
// locked" instead of waiting for the lock.
func TestNativeReconciler_RacesDownloadExactlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")+"?_busy_timeout=10000&_journal_mode=WAL"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&models.NativeResource{}))
	store := services.NewNativeResourceStore(db, db)
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{
		wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend },
		wrapService: func(s services.ServiceInterface) services.ServiceInterface {
			return ledgerAndSpendService{serviceWithNativeStore{ServiceInterface: s, store: store}, spend}
		},
	})
	raceDownloadAndReconciler(t, h, store, spend)
}

// The same race with the ledger on Postgres, where the CAS updates take row
// locks instead of SQLite's database lock.
func TestNativeReconciler_RacesDownloadExactlyOnce_Postgres(t *testing.T) {
	db := nativePostgresLedger(t)
	store := services.NewNativeResourceStore(db, db)
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{
		wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend },
		wrapService: func(s services.ServiceInterface) services.ServiceInterface {
			return ledgerAndSpendService{serviceWithNativeStore{ServiceInterface: s, store: store}, spend}
		},
	})
	raceDownloadAndReconciler(t, h, store, spend)
}

// ledgerAndSpendService swaps in a ledger and keeps the gateway's budget
// (which a plain wrapper hides from budgetServiceOf).
type ledgerAndSpendService struct {
	serviceWithNativeStore
	spend *spendCounter
}

func (s ledgerAndSpendService) GetBudgetService() services.BudgetService { return s.spend }

func raceDownloadAndReconciler(t *testing.T, h *nativeHarness, store services.NativeResourceStore, spend *spendCounter) {
	t.Helper()
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.addPrice("claude-sol", 0.00002, 0.000004)
	ctx := context.Background()
	for round := 0; round < 5; round++ {
		vendorID := fmt.Sprintf("msgbatch_RACE%d", round)
		h.vendor.addBatch(vendorID, "ended")
		h.vendor.setResults(vendorID, batchLines()...)
		row := &models.NativeResource{
			Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID, UserID: h.appA.UserID,
			ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: vendorID,
			State: "ended", AccountingState: models.NativeAccountingPending,
		}
		require.NoError(t, store.Create(ctx, row))

		gate := make(chan struct{})
		h.vendor.mu.Lock()
		h.vendor.resultsGate = gate
		h.vendor.mu.Unlock()

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, vendorID)), "")
			assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		}()
		var outcome nativeAccountOutcome
		var accErr error
		go func() {
			defer wg.Done()
			outcome, accErr = h.proxy.accountNativeRow(ctx, row)
		}()
		resultsPathV := "/v1/messages/batches/" + vendorID + "/results"
		require.Eventually(t, func() bool { return len(h.vendor.callsTo(http.MethodGet, resultsPathV)) == 2 },
			5*time.Second, 5*time.Millisecond, "both streams in flight")
		close(gate)
		wg.Wait()
		require.NoError(t, accErr)
		assert.Contains(t, []nativeAccountOutcome{nativeAccountAccounted, nativeAccountSettled}, outcome)

		got, err := store.GetByOwner(ctx, h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, vendorID)
		require.NoError(t, err)
		assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
	}
	h.vendor.mu.Lock()
	h.vendor.resultsGate = nil
	h.vendor.mu.Unlock()

	// Counted at the budget, in memory: the harness's shared-cache SQLite can
	// drop an analytics row with "table is locked" under load, so the chat
	// record count is only an upper bound.
	h.proxy.waitForAnalyzers()
	_, calls := spend.totals()
	assert.Equal(t, 10, calls, "two models per batch, five batches, each accounted once")
	assert.LessOrEqual(t, len(h.chatRecords(h.appA)), 10, "never a second set of records")
}

// Only a node whose gate is open reconciles: with the gate shut the vendor
// is never called; opening it lets the next pass account the batch. stop
// ends the workers, idempotently.
func TestNativeWorkers_GateAndStop(t *testing.T) {
	fastNativeWorkers(t)
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_W1", "ended")
	h.vendor.setResults("msgbatch_W1", succeededLine("a", "claude-luna", 10, 5))
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_W1")

	var mu sync.Mutex
	leader, asked := false, 0
	gate := func() bool {
		mu.Lock()
		defer mu.Unlock()
		asked++
		return leader
	}
	stop := h.proxy.StartNativeWorkers(context.Background(), gate)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return asked >= 5 }, 2*time.Second, 5*time.Millisecond)
	assert.Empty(t, h.vendor.recorded(), "a follower never calls the vendor")

	mu.Lock()
	leader = true
	mu.Unlock()
	require.Eventually(t, func() bool {
		return h.ledgerRow(row.ID).AccountingState == models.NativeAccountingAccounted
	}, 2*time.Second, 10*time.Millisecond)

	done := make(chan struct{})
	go func() { stop(); stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return")
	}
	calls := len(h.vendor.recorded())
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, calls, len(h.vendor.recorded()), "no vendor calls after stop")
	assert.Len(t, h.chatRecords(h.appA), 1)
}

// Without a ledger the workers are a no-op.
func TestNativeWorkers_NoStore(t *testing.T) {
	p := New(serviceWithoutNativeStore{}, nil, &Config{})
	stop := p.StartNativeWorkers(context.Background(), nil)
	stop()
}

// A stop during a slow results download returns once the download is
// cancelled, and the row is not left accounted.
func TestNativeWorkers_StopDuringDownload(t *testing.T) {
	fastNativeWorkers(t)
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_S1", "ended")
	h.vendor.setResults("msgbatch_S1", succeededLine("a", "claude-luna", 10, 5), succeededLine("b", "claude-luna", 10, 5))
	gate := make(chan struct{})
	h.vendor.mu.Lock()
	h.vendor.resultsGate = gate
	h.vendor.mu.Unlock()
	t.Cleanup(func() { close(gate) })
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_S1")

	stop := h.proxy.StartNativeWorkers(context.Background(), nil)
	require.Eventually(t, func() bool {
		return len(h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_S1/results")) == 1
	}, 2*time.Second, 5*time.Millisecond)
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stop blocked on a download")
	}
	got := h.ledgerRow(row.ID)
	assert.NotEqual(t, models.NativeAccountingAccounted, got.AccountingState)
	assert.Empty(t, h.chatRecords(h.appA))
}

// nativePostgresLedger opens a throwaway Postgres schema for the ledger when
// DATABASE_URL is set.
func nativePostgresLedger(t *testing.T) *gorm.DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL tests")
	}
	admin, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	schema := fmt.Sprintf("native_reconciler_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
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
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&models.NativeResource{}))
	return db
}

// Vendor default headers fill only what the client did not send.
func TestNativeUpstreamRequest_DefaultsDoNotOverrideClient(t *testing.T) {
	p := New(serviceWithoutNativeStore{}, nil, &Config{})
	llm := &models.LLM{Vendor: models.ANTHROPIC, APIEndpoint: "https://api.anthropic.com", APIKey: "k"}
	defaults := http.Header{"Anthropic-Version": {anthropicVendor.NativeAPIVersion}, "X-Default": {"d"}}

	req, err := p.nativeUpstreamRequest(context.Background(), llm, http.MethodGet, "/v1/messages/batches/msgbatch_1", nil, nil,
		http.Header{"Anthropic-Version": {"2099-01-01"}}, defaults)
	require.NoError(t, err)
	assert.Equal(t, "2099-01-01", req.Header.Get("anthropic-version"), "the client's version wins")
	assert.Equal(t, "d", req.Header.Get("x-default"))
	assert.Equal(t, "k", req.Header.Get("x-api-key"))
	assert.Equal(t, "https://api.anthropic.com/v1/messages/batches/msgbatch_1", req.URL.String())

	req, err = p.nativeUpstreamRequest(context.Background(), llm, http.MethodGet, "/v1/messages/batches/msgbatch_1", nil, nil, nil, defaults)
	require.NoError(t, err)
	assert.Equal(t, anthropicVendor.NativeAPIVersion, req.Header.Get("anthropic-version"))
}

// settleFlakyStore fails the first failSettles Settle calls.
type settleFlakyStore struct {
	services.NativeResourceStore
	mu          sync.Mutex
	failSettles int
	settles     int
}

func (s *settleFlakyStore) Settle(ctx context.Context, id uint, token, state string, now time.Time) (bool, error) {
	s.mu.Lock()
	s.settles++
	fail := s.settles <= s.failSettles
	s.mu.Unlock()
	if fail {
		return false, fmt.Errorf("database table is locked")
	}
	return s.NativeResourceStore.Settle(ctx, id, token, state, now)
}

// A transient store error on the settle is retried while the claim is still
// ours; one that persists is a failure (never an "accounted" outcome) and
// records nothing.
func TestNativeReconciler_SettleStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failures int
		want     nativeAccountOutcome
		wantErr  bool
		state    string
		records  int
	}{
		{"transient", nativeSettleAttempts - 1, nativeAccountAccounted, false, models.NativeAccountingAccounted, 1},
		{"persistent", nativeSettleAttempts, nativeAccountFailed, true, models.NativeAccountingClaimed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &settleFlakyStore{failSettles: tc.failures}
			h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
				store.NativeResourceStore = s.(*services.Service).NativeResourceStore()
				return serviceWithNativeStore{ServiceInterface: s, store: store}
			}})
			h.addPrice("claude-luna", 0.00001, 0.000002)
			h.vendor.addBatch("msgbatch_F1", "ended")
			h.vendor.setResults("msgbatch_F1", succeededLine("a", "claude-luna", 10, 5))
			row := h.seedBatchRow(h.llm, h.appA, "msgbatch_F1")

			out, err := h.proxy.accountNativeRow(context.Background(), row)
			assert.Equal(t, tc.want, out)
			assert.Equal(t, tc.wantErr, err != nil, "err = %v", err)
			assert.Equal(t, tc.state, h.ledgerRow(row.ID).AccountingState)
			if tc.records > 0 {
				require.Eventually(t, func() bool { return len(h.chatRecords(h.appA)) == tc.records }, 5*time.Second, 20*time.Millisecond)
			} else {
				assert.Empty(t, h.chatRecords(h.appA))
			}
		})
	}
	assert.Equal(t, "failed", nativeAccountFailed.String())
	assert.Equal(t, nativeAccountFailed, nativeAccountOutcome(0), "the zero outcome is a failure")
}

// A results line over the size limit does not hold the batch's accounting
// back until its results expire: the reconciler skips it and accounts the
// other lines on its first pass.
func TestNativeReconciler_OversizedLineSkipped(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_BIG", "ended")
	huge := `{"custom_id":"huge","result":{"type":"succeeded","message":{"content":[{"type":"text","text":"` +
		strings.Repeat("x", nativeapi.DefaultMaxLineSize+1) + `"}]}}}`
	h.vendor.setResults("msgbatch_BIG",
		succeededLine("a", "claude-luna", 1000, 200), huge, succeededLine("b", "claude-luna", 500, 100))
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_BIG")

	out, err := h.proxy.accountNativeRow(context.Background(), row)
	require.NoError(t, err)
	assert.Equal(t, nativeAccountAccounted, out, "settled on the first pass")
	assert.Equal(t, models.NativeAccountingAccounted, h.ledgerRow(row.ID).AccountingState)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool { recs = h.chatRecords(h.appA); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, 1500, recs[0].PromptTokens, "the lines around the oversized one are accounted")
	assert.Equal(t, 300, recs[0].ResponseTokens)
}

// Unset (nil or zero) reconciler settings keep today's defaults exactly;
// set ones override them, and the max backoff never drops below the poll
// interval.
func TestNativeReconcilerConfig(t *testing.T) {
	for _, c := range []*Config{nil, {}} {
		assert.Equal(t, 30*time.Second, c.nativeReconcileInterval())
		assert.Equal(t, 50, c.nativeReconcileBatch())
		assert.Equal(t, 4, c.nativeReconcileWorkers())
		assert.Equal(t, 2*time.Minute, c.nativePollInterval())
		assert.Equal(t, 30*time.Minute, c.nativePollMaxBackoff())
		assert.Equal(t, 2*time.Minute, c.nativePollBackoff(1))
		assert.Equal(t, 16*time.Minute, c.nativePollBackoff(4))
		assert.Equal(t, 30*time.Minute, c.nativePollBackoff(5))
		assert.Equal(t, 30*time.Minute, c.nativePollBackoff(1000))
	}

	c := &Config{NativeReconcileInterval: 45 * time.Second, NativeReconcileBatch: 7, NativeReconcileWorkers: 2,
		NativePollInterval: 5 * time.Minute, NativePollMaxBackoff: 12 * time.Minute}
	assert.Equal(t, 45*time.Second, c.nativeReconcileInterval())
	assert.Equal(t, 7, c.nativeReconcileBatch())
	assert.Equal(t, 2, c.nativeReconcileWorkers())
	assert.Equal(t, 5*time.Minute, c.nativePollInterval())
	assert.Equal(t, []time.Duration{5 * time.Minute, 10 * time.Minute, 12 * time.Minute, 12 * time.Minute},
		[]time.Duration{c.nativePollBackoff(1), c.nativePollBackoff(2), c.nativePollBackoff(3), c.nativePollBackoff(9)})

	low := &Config{NativePollInterval: time.Hour, NativePollMaxBackoff: time.Minute}
	assert.Equal(t, time.Hour, low.nativePollMaxBackoff(), "the backoff cap is never below the poll interval")
	assert.Equal(t, time.Hour, low.nativePollBackoff(3))
	neg := &Config{NativeReconcileInterval: -time.Second, NativeReconcileBatch: -1, NativeReconcileWorkers: -1,
		NativePollInterval: -time.Second, NativePollMaxBackoff: -time.Second}
	assert.Equal(t, 30*time.Second, neg.nativeReconcileInterval())
	assert.Equal(t, 50, neg.nativeReconcileBatch())
	assert.Equal(t, 4, neg.nativeReconcileWorkers())
	assert.Equal(t, 30*time.Minute, neg.nativePollMaxBackoff())
}

// The running reconciler uses the configured timings: the pass interval,
// rows per pass, and the next poll of an unfinished batch.
func TestNativeWorkers_UseConfiguredTimings(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{config: &Config{
		NativeReconcileInterval: 10 * time.Millisecond,
		NativeReconcileBatch:    1,
		NativePollInterval:      7 * time.Minute,
	}})
	for _, id := range []string{"msgbatch_C1", "msgbatch_C2"} {
		h.vendor.addBatch(id, "in_progress")
	}
	r1 := h.seedBatchRow(h.llm, h.appA, "msgbatch_C1")
	r2 := h.seedBatchRow(h.llm, h.appA, "msgbatch_C2")

	// One row per pass: a single pass polls only the first due row.
	start := time.Now()
	h.proxy.reconcileNative(context.Background(), h.service.NativeResourceStore(), start)
	assert.Len(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_C1"), 1)
	assert.Empty(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_C2"))
	got := h.ledgerRow(r1.ID)
	require.NotNil(t, got.NextPollAt)
	assert.WithinDuration(t, start.Add(7*time.Minute), *got.NextPollAt, time.Second, "configured poll interval")

	// The 10ms pass interval (the default is 30s) reaches the second row
	// quickly; the package defaults are untouched.
	stop := h.proxy.StartNativeWorkers(context.Background(), nil)
	defer stop()
	// The worker sets NextPollAt only after the vendor's GET returns, so
	// wait for the row, not just the GET. (ledgerRow uses require, which
	// must not run on Eventually's goroutine.)
	require.Eventually(t, func() bool {
		if len(h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_C2")) == 0 {
			return false
		}
		var row models.NativeResource
		return h.db.First(&row, r2.ID).Error == nil && row.NextPollAt != nil
	}, 5*time.Second, 5*time.Millisecond, "the second row was polled and its next poll scheduled")
	assert.Equal(t, 30*time.Second, nativeReconcileInterval)
}
