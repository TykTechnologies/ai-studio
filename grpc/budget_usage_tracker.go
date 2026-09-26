package grpc

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
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
// llm_chat_records rows are only ever inserted, never updated or deleted.
type budgetUsageTracker struct {
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
}

type periodKey struct{ start, end int64 }

// appPeriodUsage is an App's spend in its current budget period.
type appPeriodUsage struct {
	cost       float64 // llm_chat_records cost units (dollars * 10000)
	start, end time.Time
}

func newBudgetUsageTracker() *budgetUsageTracker {
	return &budgetUsageTracker{totals: map[uint]*periodSpend{}}
}

// usage returns each App's spend for its current budget period at now.
func (t *budgetUsageTracker) usage(db *gorm.DB, apps []models.App, now time.Time) (map[uint]appPeriodUsage, error) {
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
			sums, err := sumSpend(db, chunk, g.start, g.end, 0, t.settledID, true)
			if err != nil {
				return nil, err
			}
			for _, id := range chunk {
				t.totals[id] = &periodSpend{start: g.start, end: g.end, settled: sums[id]}
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
				sums, err := sumSpend(db, nil, g.start, g.end, t.settledID, target, true)
				if err != nil {
					return nil, err
				}
				for _, id := range g.apps {
					t.totals[id].settled += sums[id]
				}
			}
			t.settledID = target
		}
	}

	// Add the rows since settledID, summed afresh.
	usage := make(map[uint]appPeriodUsage, len(apps))
	for _, g := range groups {
		recent, err := sumSpend(db, nil, g.start, g.end, t.settledID, 0, false)
		if err != nil {
			return nil, err
		}
		for _, id := range g.apps {
			usage[id] = appPeriodUsage{cost: t.totals[id].settled + recent[id], start: g.start, end: g.end}
		}
	}
	return usage, nil
}

// sumSpend sums llm_chat_records cost per App within [start, end], for rows
// with id > afterID and, when bounded, id <= uptoID; optionally only for
// appIDs. Ids start at 1, so afterID 0 is no lower bound.
func sumSpend(db *gorm.DB, appIDs []uint, start, end time.Time, afterID, uptoID uint, bounded bool) (map[uint]float64, error) {
	q := db.Model(&models.LLMChatRecord{}).
		Select("app_id, COALESCE(SUM(cost), 0) AS total").
		Where("time_stamp >= ? AND time_stamp <= ?", start, end)
	if appIDs != nil {
		q = q.Where("app_id IN ?", appIDs)
	}
	if afterID > 0 {
		q = q.Where("id > ?", afterID)
	}
	if bounded {
		q = q.Where("id <= ?", uptoID)
	}
	var rows []struct {
		AppID uint
		Total float64
	}
	if err := q.Group("app_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	sums := make(map[uint]float64, len(rows))
	for _, r := range rows {
		sums[r.AppID] = r.Total
	}
	return sums, nil
}
