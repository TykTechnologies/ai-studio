package services

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ledgerRefreshInterval is how often the analytics writer re-reads the stored
// usage of the ledger's entries. The re-read picks up writes the ledger did not
// make: another gateway on a shared database, or a budget sync.
const ledgerRefreshInterval = 5 * time.Second

// edgeBudgetLedger is the gateway's ledger, shared by the budget service and
// the budget sync handler; nil until StartAnalyticsWriter runs.
var edgeBudgetLedger atomic.Pointer[BudgetLedger]

// BudgetLedger keeps each App's budget usage for the current period in memory.
//
// Recording usage adds to the ledger instead of writing budget_usage on every
// request; the analytics writer applies the accumulated amounts in its batch
// transaction, as increments, so several gateways can share one database. The
// budget check reads the ledger, which includes spend not yet written, so
// enforcement sees a request's cost as soon as it is recorded.
//
// The request path reads the database only for an App's first check in a
// period, and concurrent first checks share that read. Later re-reads run on
// the writer's goroutine (refresh), between its flushes.
type BudgetLedger struct {
	db *gorm.DB // reads budget_usage

	mu      sync.Mutex
	entries map[ledgerKey]*ledgerEntry
	now     func() time.Time
}

type ledgerKey struct {
	appID uint
	start int64 // period start, unix nanoseconds
	end   int64
}

// usageTotals are amounts in budget_usage's units (cost is dollars * 10000).
type usageTotals struct {
	tokens, requests, prompt, completion int64
	cost                                 float64
}

func (u *usageTotals) add(o usageTotals) {
	u.tokens += o.tokens
	u.requests += o.requests
	u.prompt += o.prompt
	u.completion += o.completion
	u.cost += o.cost
}

func (u usageTotals) zero() bool { return u == usageTotals{} }

type ledgerEntry struct {
	appID      uint
	start, end time.Time

	// base is the stored total cost as last read or written.
	base   float64
	loaded bool
	// seeding is closed when a running first read finishes.
	seeding chan struct{}
	pending usageTotals // recorded, not yet handed to a flush
	// inflight is what a running flush is writing; flushes counts the
	// flushes started and finished, to detect a read that overlapped one.
	inflight usageTotals
	flushes  uint64
}

func (e *ledgerEntry) spent() float64 { return e.base + e.inflight.cost + e.pending.cost }

// NewBudgetLedger returns an empty ledger reading stored usage from db.
func NewBudgetLedger(db *gorm.DB) *BudgetLedger {
	return &BudgetLedger{db: db, entries: map[ledgerKey]*ledgerEntry{}, now: time.Now}
}

func keyFor(appID uint, start, end time.Time) ledgerKey {
	return ledgerKey{appID: appID, start: start.UnixNano(), end: end.UnixNano()}
}

// entry returns the App's entry for the period, creating it; l.mu is held.
func (l *BudgetLedger) entry(appID uint, start, end time.Time) *ledgerEntry {
	k := keyFor(appID, start, end)
	e := l.entries[k]
	if e == nil {
		e = &ledgerEntry{appID: appID, start: start, end: end}
		l.entries[k] = e
	}
	return e
}

// Spent returns the App's spend for the period in budget_usage's units: the
// stored total plus everything recorded since. The stored total is read from
// the database the first time only; concurrent first calls share the read.
// An error means that read failed.
func (l *BudgetLedger) Spent(appID uint, start, end time.Time) (float64, error) {
	l.mu.Lock()
	e := l.entry(appID, start, end)
	if e.loaded {
		spent := e.spent()
		l.mu.Unlock()
		return spent, nil
	}
	if wait := e.seeding; wait != nil {
		l.mu.Unlock()
		<-wait
		l.mu.Lock()
		defer l.mu.Unlock()
		if !e.loaded {
			return 0, errors.New("reading stored budget usage failed")
		}
		return e.spent(), nil
	}
	done := make(chan struct{})
	e.seeding = done
	flushes := e.flushes
	l.mu.Unlock()

	stored, err := l.readStored(appID, start, end)

	l.mu.Lock()
	defer l.mu.Unlock()
	e.seeding = nil
	close(done)
	if err != nil {
		return 0, err
	}
	if e.flushes == flushes && e.inflight.zero() {
		e.base = stored
	} else if stored > e.base {
		// A flush overlapped the read, which may or may not include
		// it; never go below either figure. The writer's next refresh
		// settles it.
		e.base = stored
	}
	e.loaded = true
	return e.spent(), nil
}

// refresh re-reads the stored total of every loaded entry. The writer calls
// it between flushes, so no flush is in flight.
func (l *BudgetLedger) refresh() {
	l.mu.Lock()
	entries := make([]*ledgerEntry, 0, len(l.entries))
	for _, e := range l.entries {
		if e.loaded {
			entries = append(entries, e)
		}
	}
	l.mu.Unlock()

	for _, e := range entries {
		stored, err := l.readStored(e.appID, e.start, e.end)
		if err != nil {
			continue
		}
		l.mu.Lock()
		if e.inflight.zero() {
			e.base = stored
		}
		l.mu.Unlock()
	}
}

func (l *BudgetLedger) readStored(appID uint, start, end time.Time) (float64, error) {
	var usage database.BudgetUsage
	err := l.db.Where("app_id = ? AND period_start = ? AND period_end = ?", appID, start, end).First(&usage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return usage.TotalCost, nil
}

// Add records one request's usage.
func (l *BudgetLedger) Add(appID uint, start, end time.Time, tokens int64, cost float64, promptTokens, completionTokens int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entry(appID, start, end)
	e.pending.add(usageTotals{tokens: tokens, requests: 1, prompt: promptTokens, completion: completionTokens, cost: cost})
}

// Reconcile raises the App's stored total for the period starting at start to
// at least cost, as the budget sync does with the control plane's figure.
func (l *BudgetLedger) Reconcile(appID uint, start time.Time, cost float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.appID == appID && e.start.Equal(start) && e.loaded && e.base < cost {
			e.base = cost
		}
	}
}

// ledgerFlush is the set of amounts one Flush writes.
type ledgerFlush struct {
	entries []*ledgerEntry
	amounts []usageTotals
}

// take moves every entry's pending amounts to inflight and returns them.
// Entries of past periods with nothing left to write are dropped.
func (l *BudgetLedger) take() *ledgerFlush {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := &ledgerFlush{}
	now := l.now()
	for k, e := range l.entries {
		if e.pending.zero() {
			if e.inflight.zero() && now.After(e.end) {
				delete(l.entries, k)
			}
			continue
		}
		e.inflight = e.pending
		e.pending = usageTotals{}
		e.flushes++
		f.entries = append(f.entries, e)
		f.amounts = append(f.amounts, e.inflight)
	}
	return f
}

// finish settles a flush: on success the written amounts join the stored
// total, otherwise they go back to pending for the next flush.
func (l *BudgetLedger) finish(f *ledgerFlush, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, e := range f.entries {
		if ok {
			e.base += f.amounts[i].cost
		} else {
			e.pending.add(f.amounts[i])
		}
		e.inflight = usageTotals{}
		e.flushes++
	}
}

// write applies the flush's amounts to budget_usage inside tx, as increments.
// A nil or empty flush writes nothing.
func (f *ledgerFlush) write(tx *gorm.DB) error {
	if f.empty() {
		return nil
	}
	now := time.Now()
	for i, e := range f.entries {
		a := f.amounts[i]
		// Create the period's row if it is missing, then add to it.
		row := database.BudgetUsage{AppID: e.appID, PeriodStart: e.start, PeriodEnd: e.end, CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		err := tx.Model(&database.BudgetUsage{}).
			Where("app_id = ? AND period_start = ? AND period_end = ?", e.appID, e.start, e.end).
			Updates(map[string]interface{}{
				"tokens_used":       gorm.Expr("tokens_used + ?", a.tokens),
				"requests_count":    gorm.Expr("requests_count + ?", a.requests),
				"total_cost":        gorm.Expr("total_cost + ?", a.cost),
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", a.prompt),
				"completion_tokens": gorm.Expr("completion_tokens + ?", a.completion),
				"updated_at":        now,
			}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *ledgerFlush) empty() bool { return f == nil || len(f.entries) == 0 }
