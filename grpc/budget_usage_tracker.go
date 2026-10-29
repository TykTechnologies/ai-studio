package grpc

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// budgetSettleCycles is how many sync cycles a chat record's id must have
// been visible before its cost is folded into an App's settled total. Rows
// inside that window are summed afresh every cycle, so one whose transaction
// commits after rows with higher ids (several hub replicas writing to one
// database) is still counted once it commits within the window.
const budgetSettleCycles = 2

// appSpendIDChunk bounds the app_id IN (...) list of one query.
const appSpendIDChunk = 500

// budgetUsageTracker keeps each App's spend for its current budget period.
//
// The budget sync used to sum llm_chat_records over every App's whole period
// on every cycle (every 30 s): its cost grew with the requests in the period,
// and at ~80M rows one pass took longer than the interval, so the hub's
// database was scanning continuously. The tracker sums an App's period once
// (at start-up, or when the period changes: a new month or a budget reset)
// and afterwards reads only the rows added since the last cycle, found by id.
// llm_chat_records rows are only inserted, except that a price
// recalculation rewrites their cost: BudgetSyncService then starts its
// trackers again (replicas.SignalSpendRecalculated).
type budgetUsageTracker struct {
	// column is the llm_chat_records column spend is kept by: app_id, or
	// llm_id for LLM budgets (ap6).
	column      string
	initialized bool
	// Rows with id <= settledID are included in the settled totals.
	settledID uint
	// The highest id seen at each recent cycle, oldest first.
	recentMax []uint
	totals    map[uint]*periodSpend
}

type periodSpend struct {
	start, end time.Time
	settled    float64 // llm_chat_records cost units (dollars * 10000)
	// otherSettled is settled spend in currencies other than the budget
	// currency, which is not counted (budget.Currency), by currency.
	otherSettled map[string]float64
}

type periodKey struct{ start, end int64 }

// appPeriodUsage is an App's spend in its current budget period.
type appPeriodUsage struct {
	cost       float64 // llm_chat_records cost units (dollars * 10000), budget currency only
	start, end time.Time
	// other is the spend in other currencies, by currency, in cost units;
	// nil when there is none. A budget with any cannot be enforced.
	other map[string]float64
}

func newBudgetUsageTracker() *budgetUsageTracker {
	return &budgetUsageTracker{column: "app_id", totals: map[uint]*periodSpend{}}
}

// newLLMBudgetUsageTracker keeps each LLM's spend for its budget period.
func newLLMBudgetUsageTracker() *budgetUsageTracker {
	return &budgetUsageTracker{column: "llm_id", totals: map[uint]*periodSpend{}}
}

// budgetOwner is what the tracker needs of an App or LLM.
type budgetOwner struct {
	ID              uint
	BudgetStartDate *time.Time
}

// usage returns each App's spend for its current budget period at now.
func (t *budgetUsageTracker) usage(db *gorm.DB, apps []models.App, now time.Time) (map[uint]appPeriodUsage, error) {
	owners := make([]budgetOwner, len(apps))
	for i, app := range apps {
		owners[i] = budgetOwner{ID: app.ID, BudgetStartDate: app.BudgetStartDate}
	}
	return t.usageOf(db, owners, now)
}

// llmUsage is usage for LLMs.
func (t *budgetUsageTracker) llmUsage(db *gorm.DB, llms []models.LLM, now time.Time) (map[uint]appPeriodUsage, error) {
	owners := make([]budgetOwner, len(llms))
	for i, llm := range llms {
		owners[i] = budgetOwner{ID: llm.ID, BudgetStartDate: llm.BudgetStartDate}
	}
	return t.usageOf(db, owners, now)
}

func (t *budgetUsageTracker) usageOf(db *gorm.DB, apps []budgetOwner, now time.Time) (map[uint]appPeriodUsage, error) {
	var maxID uint
	if err := db.Model(&models.LLMChatRecord{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
		return nil, err
	}
	if !t.initialized {
		t.settledID = maxID
		t.initialized = true
	}

	// Group the Apps by budget period: one query per distinct period.
	type group struct {
		start, end time.Time
		apps       []uint
	}
	groups := map[periodKey]*group{}
	present := make(map[uint]bool, len(apps))
	for _, app := range apps {
		start, end := calculateBudgetPeriod(app.BudgetStartDate, now)
		k := periodKey{start.UnixNano(), end.UnixNano()}
		g := groups[k]
		if g == nil {
			g = &group{start: start, end: end}
			groups[k] = g
		}
		g.apps = append(g.apps, app.ID)
		present[app.ID] = true
	}
	for id := range t.totals {
		if !present[id] {
			delete(t.totals, id)
		}
	}

	// Apps seen for the first time, or whose period changed, get their
	// period summed up to settledID.
	for _, g := range groups {
		var fresh []uint
		for _, id := range g.apps {
			if p := t.totals[id]; p == nil || !p.start.Equal(g.start) || !p.end.Equal(g.end) {
				fresh = append(fresh, id)
			}
		}
		for i := 0; i < len(fresh); i += appSpendIDChunk {
			chunk := fresh[i:min(i+appSpendIDChunk, len(fresh))]
			sums, err := sumSpend(db, t.column, chunk, g.start, g.end, 0, t.settledID, true)
			if err != nil {
				return nil, err
			}
			for _, id := range chunk {
				t.totals[id] = &periodSpend{start: g.start, end: g.end, settled: sums[id].budget, otherSettled: sums[id].other}
			}
		}
	}

	// Fold rows that have been visible for budgetSettleCycles into the
	// settled totals.
	t.recentMax = append(t.recentMax, maxID)
	if len(t.recentMax) > budgetSettleCycles {
		target := t.recentMax[0]
		t.recentMax = t.recentMax[1:]
		if target > t.settledID {
			for _, g := range groups {
				sums, err := sumSpend(db, t.column, nil, g.start, g.end, t.settledID, target, true)
				if err != nil {
					return nil, err
				}
				for _, id := range g.apps {
					t.totals[id].settled += sums[id].budget
					t.totals[id].otherSettled = addOther(t.totals[id].otherSettled, sums[id].other)
				}
			}
			t.settledID = target
		}
	}

	// Add the rows since settledID, summed afresh.
	usage := make(map[uint]appPeriodUsage, len(apps))
	for _, g := range groups {
		recent, err := sumSpend(db, t.column, nil, g.start, g.end, t.settledID, 0, false)
		if err != nil {
			return nil, err
		}
		for _, id := range g.apps {
			p := t.totals[id]
			usage[id] = appPeriodUsage{
				cost: p.settled + recent[id].budget, start: g.start, end: g.end,
				other: addOther(addOther(nil, p.otherSettled), recent[id].other),
			}
		}
	}
	return usage, nil
}

// sumSpend sums llm_chat_records cost per App (per column: app_id or
// llm_id) within the period start..end (end is the period's last second, so
// rows up to the next period's start count), for rows with id > afterID and, when bounded,
// id <= uptoID; optionally only for appIDs. Ids start at 1, so afterID 0 is
// no lower bound. Spend in the budget currency and in other currencies is
// summed apart.
func sumSpend(db *gorm.DB, column string, appIDs []uint, start, end time.Time, afterID, uptoID uint, bounded bool) (map[uint]spendSums, error) {
	q := db.Model(&models.LLMChatRecord{}).
		Select(column+" AS app_id, "+budget.CurrencyClassSQL+" AS currency_class, COALESCE(SUM(cost), 0) AS total").
		Where("time_stamp >= ? AND time_stamp < ?", start, end.Add(time.Second))
	if appIDs != nil {
		q = q.Where(column+" IN ?", appIDs)
	}
	if afterID > 0 {
		q = q.Where("id > ?", afterID)
	}
	if bounded {
		q = q.Where("id <= ?", uptoID)
	}
	var rows []struct {
		AppID         uint
		CurrencyClass string
		Total         float64
	}
	if err := q.Group(column + ", currency_class").Scan(&rows).Error; err != nil {
		return nil, err
	}
	sums := make(map[uint]spendSums, len(rows))
	for _, r := range rows {
		s := sums[r.AppID]
		if r.CurrencyClass == "" {
			s.budget += r.Total
		} else {
			s.other = addOther(s.other, map[string]float64{r.CurrencyClass: r.Total})
		}
		sums[r.AppID] = s
	}
	return sums, nil
}

// spendSums is one App's (or LLM's) spend in cost units: in the budget
// currency, and in other currencies by currency.
type spendSums struct {
	budget float64
	other  map[string]float64
}

// addOther adds b's amounts to a, allocating a when needed; amounts of 0 are
// left out.
func addOther(a, b map[string]float64) map[string]float64 {
	for c, v := range b {
		if v == 0 {
			continue
		}
		if a == nil {
			a = map[string]float64{}
		}
		a[c] += v
	}
	return a
}
