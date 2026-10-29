package proxy

// Pricing regression suite: model price changes against recorded history.
// Months of multi-vendor traffic are recorded through the real
// buildChatRecordFor path, prices change, and the tests assert what the cost
// chart and the chat records show. Every test runs on SQLite and, when
// DATABASE_URL is a Postgres URL, on Postgres in a schema of its own (CI's Go
// Postgres Tests job runs them there).
//
// The normal run uses a short, thinned history to stay cheap. Set
// PRICING_REGRESSION_FULL=1 for the full matrix (180 days at full traffic,
// 180k rows recalculated); make test-pricing runs that on SQLite and a
// throwaway postgres:16. The log-only diagnostics these fixtures come from
// are in pricing_stress_diag_test.go (pricing_stress tag).

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stressModel struct {
	vendor    models.Vendor
	requested string // what the caller (agent) asks for
	reported  string // what the vendor reports back (Gemini modelVersion, OpenAI snapshot)
	perDay    int
	prompt    int
	response  int
	cacheRead int
	price     models.ModelPrice // the real price the admin eventually enters
}

// Gemini mix as an agent would use it: one model reported under the name
// asked for, two reported under a versioned name.
var geminiMix = []stressModel{
	{vendor: models.GOOGLEAI, requested: "gemini-2.5-pro", reported: "gemini-2.5-pro", perDay: 20, prompt: 12000, response: 1500, cacheRead: 4000,
		price: models.ModelPrice{CPIT: 1.25e-6, CPT: 10e-6, CacheReadPT: 0.31e-6, Currency: "USD"}},
	{vendor: models.GOOGLEAI, requested: "gemini-2.5-flash", reported: "gemini-2.5-flash-preview-05-20", perDay: 80, prompt: 8000, response: 900, cacheRead: 2000,
		price: models.ModelPrice{CPIT: 0.3e-6, CPT: 2.5e-6, CacheReadPT: 0.075e-6, Currency: "USD"}},
	{vendor: models.GOOGLEAI, requested: "gemini-2.0-flash", reported: "gemini-2.0-flash-001", perDay: 120, prompt: 3000, response: 400,
		price: models.ModelPrice{CPIT: 0.1e-6, CPT: 0.4e-6, Currency: "USD"}},
}

var otherVendors = []stressModel{
	{vendor: models.OPENAI, requested: "gpt-4o", reported: "gpt-4o-2024-08-06", perDay: 40, prompt: 5000, response: 700, cacheRead: 1000,
		price: models.ModelPrice{CPIT: 2.5e-6, CPT: 10e-6, CacheReadPT: 1.25e-6, Currency: "USD"}},
	{vendor: models.ANTHROPIC, requested: "claude-sonnet-4-5", reported: "claude-sonnet-4-5-20250929", perDay: 30, prompt: 9000, response: 1200, cacheRead: 6000,
		price: models.ModelPrice{CPIT: 3e-6, CPT: 15e-6, CacheWritePT: 3.75e-6, CacheReadPT: 0.3e-6, Currency: "EUR"}},
}

// pricingFull is set for the full matrix (make test-pricing).
var pricingFull = os.Getenv("PRICING_REGRESSION_FULL") != ""

// pricingScale is the history length and the traffic mix for this run: the
// full 180 days, or four weeks at a tenth of the traffic.
func pricingScale(mix []stressModel) (days int, scaled []stressModel) {
	if pricingFull {
		return 180, mix
	}
	for _, m := range mix {
		m.perDay = max(1, m.perDay/10)
		scaled = append(scaled, m)
	}
	return 28, scaled
}

type stressEnv struct {
	db   *gorm.DB
	svc  *services.Service
	llms map[models.Vendor]*models.LLM
	app  *models.App
	day0 time.Time // midnight UTC of the first day
	days int
}

func newStressEnvOn(t *testing.T, db *gorm.DB, days int) *stressEnv {
	env := &stressEnv{db: db, svc: &services.Service{DB: db}, llms: map[models.Vendor]*models.LLM{}, days: days}
	for _, v := range []models.Vendor{models.GOOGLEAI, models.OPENAI, models.ANTHROPIC} {
		llm := &models.LLM{Name: fmt.Sprintf("llm-%s", v), Vendor: v}
		require.NoError(t, db.Create(llm).Error)
		env.llms[v] = llm
	}
	env.app = &models.App{Name: "agent"}
	require.NoError(t, env.app.Create(db)) // with a credential, which Postgres enforces
	today := time.Now().UTC().Truncate(24 * time.Hour)
	env.day0 = today.AddDate(0, 0, -(days - 1))
	return env
}

// forEachPricingDB runs fn on SQLite and on Postgres (a schema of its own;
// skipped unless DATABASE_URL is a Postgres URL).
func forEachPricingDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, setupTestDB(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, pricingPostgresDB(t)) })
}

func pricingPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(dsn, "postgres") {
		t.Skip("DATABASE_URL is not a Postgres URL")
	}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), cfg)
	require.NoError(t, err)
	schema := fmt.Sprintf("pricing_regression_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
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
	require.NoError(t, models.InitModels(db))
	require.NoError(t, analytics.Migrate(db))
	return db
}

// record sends one day's traffic for the mix through buildChatRecordFor,
// spread over the day.
func (e *stressEnv) record(t *testing.T, mix []stressModel, day int) {
	t.Helper()
	var recs []*models.LLMChatRecord
	for _, m := range mix {
		for i := 0; i < m.perDay; i++ {
			ts := e.day0.AddDate(0, 0, day).Add(time.Duration(i) * (23 * time.Hour) / time.Duration(m.perDay))
			resp := &MockTokenResponse{model: m.reported, promptTokens: m.prompt + m.cacheRead, responseTokens: m.response, cacheReadTokens: m.cacheRead}
			recs = append(recs, buildChatRecordFor(e.svc, e.llms[m.vendor], e.app, resp, m.requested, ts))
		}
	}
	require.NoError(t, e.db.CreateInBatches(recs, 500).Error)
}

func (e *stressEnv) recordDays(t *testing.T, mix []stressModel, from, to int) {
	for d := from; d < to; d++ {
		e.record(t, mix, d)
	}
}

// dayCost is what one day of m costs at its real price, in dollars.
func (m stressModel) dayCost() float64 {
	return m.dayCostAt(m.price)
}

// dayCostAt is what one day of m costs at price p.
func (m stressModel) dayCostAt(p models.ModelPrice) float64 {
	pt := models.PromptTokensExcludingCache(string(m.vendor), m.prompt+m.cacheRead, 0, m.cacheRead)
	c := models.ChatRecordCost(string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, pt, m.response, 0, m.cacheRead)
	return c * float64(m.perDay) / 10000
}

func priceRow(t *testing.T, db *gorm.DB, name string, vendor models.Vendor) *models.ModelPrice {
	t.Helper()
	var p models.ModelPrice
	require.NoError(t, db.Where("model_name = ? AND vendor = ?", name, string(vendor)).First(&p).Error)
	return &p
}

func (e *stressEnv) createPrices(t *testing.T, mix []stressModel) {
	t.Helper()
	for _, m := range mix {
		p := m.price
		_, err := e.svc.CreateModelPrice(m.reported, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
		require.NoError(t, err)
	}
}

type dailyCost struct {
	byDay map[string]float64
	total float64
}

// costByDay is GetCostAnalysis (the dashboard's cost chart) folded over currencies.
func (e *stressEnv) costByDay(t *testing.T) dailyCost {
	t.Helper()
	out := dailyCost{byDay: map[string]float64{}}
	for _, d := range e.costChart(t) {
		for day, v := range d.byDay {
			out.byDay[day] += v
			out.total += v
		}
	}
	return out
}

// costChart is GetCostAnalysis per currency, keyed by day.
func (e *stressEnv) costChart(t *testing.T) map[string]dailyCost {
	t.Helper()
	chart, err := analytics.GetCostAnalysis(e.db, e.day0, e.day0.AddDate(0, 0, e.days+1), nil)
	require.NoError(t, err)
	out := map[string]dailyCost{}
	for cur, c := range chart {
		d := dailyCost{byDay: map[string]float64{}}
		for i, l := range c.Labels {
			// DATE() scans as "2006-01-02" on SQLite and as an RFC 3339
			// midnight on Postgres.
			day := l[:min(len(l), 10)]
			d.byDay[day] += c.Data[i]
			d.total += c.Data[i]
		}
		out[cur] = d
	}
	return out
}

// modelCostByDay is one model's recorded cost per UTC day, in dollars.
func (e *stressEnv) modelCostByDay(t *testing.T, name string) map[string]float64 {
	t.Helper()
	var rows []struct {
		TimeStamp time.Time
		Cost      float64
	}
	require.NoError(t, e.db.Model(&models.LLMChatRecord{}).Select("time_stamp, cost").Where("name = ?", name).Scan(&rows).Error)
	out := map[string]float64{}
	for _, r := range rows {
		out[r.TimeStamp.UTC().Format("2006-01-02")] += r.Cost / 10000
	}
	return out
}

func (e *stressEnv) dayLabel(day int) string { return e.day0.AddDate(0, 0, day).Format("2006-01-02") }

func (e *stressEnv) lastDay() string { return e.dayLabel(e.days - 1) }

func mixDayCost(mix []stressModel) float64 {
	s := 0.0
	for _, m := range mix {
		s += m.dayCost()
	}
	return s
}

// assertRel fails unless got is within rel of want (relative error).
func assertRel(t *testing.T, want, got, rel float64, msgAndArgs ...any) {
	t.Helper()
	if want == 0 {
		assert.InDelta(t, 0, got, 1e-12, msgAndArgs...)
		return
	}
	assert.InEpsilon(t, want, got, rel, msgAndArgs...)
}

// Traffic priced from day 0 on the name the vendor reports: every day of
// the cost chart shows each currency's cost at the real price, and nothing
// lands on the wrong day or in the wrong currency.
func TestPricingRegression_PricedHistoryChart(t *testing.T) {
	forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
		days, mix := pricingScale(append(append([]stressModel{}, geminiMix...), otherVendors...))
		env := newStressEnvOn(t, db, days)
		env.createPrices(t, mix)
		env.recordDays(t, mix, 0, days)

		want := map[string]float64{}
		for _, m := range mix {
			want[m.price.Currency] += m.dayCost()
		}
		chart := env.costChart(t)
		require.Len(t, chart, len(want), "one chart series per currency")
		for cur, perDay := range want {
			got := chart[cur]
			require.Len(t, got.byDay, days, "%s: a point for every day", cur)
			for d := 0; d < days; d++ {
				assertRel(t, perDay, got.byDay[env.dayLabel(d)], 1e-9, "%s cost on day %d", cur, d)
			}
		}
	})
}

// A mid-history vendor price change (standard update), then a recalculation
// that only nudges a cache price: each interval keeps the price it was
// recorded at, moved by no more than the nudge.
func TestPricingRegression_RecalcKeepsIntervals(t *testing.T) {
	forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
		days, mix := pricingScale(append(append([]stressModel{}, geminiMix...), otherVendors...))
		change := days / 2
		env := newStressEnvOn(t, db, days)
		env.createPrices(t, mix)
		env.recordDays(t, mix, 0, change)

		flash := mix[1]
		halved := flash.price
		halved.CPT, halved.CPIT, halved.CacheReadPT = halved.CPT/2, halved.CPIT/2, halved.CacheReadPT/2
		row := priceRow(t, env.db, flash.reported, flash.vendor)
		_, err := env.svc.UpdateModelPrice(row.ID, flash.reported, string(flash.vendor), halved.CPT, halved.CPIT, 0, halved.CacheReadPT, "USD")
		require.NoError(t, err)
		env.recordDays(t, mix, change, days)

		before := env.modelCostByDay(t, flash.reported)
		for d := 0; d < days; d++ {
			want := flash.dayCost()
			if d >= change {
				want = flash.dayCostAt(halved)
			}
			assertRel(t, want, before[env.dayLabel(d)], 1e-9, "day %d before the recalculation", d)
		}
		others := map[string]map[string]float64{}
		for _, m := range mix {
			if m.reported != flash.reported {
				others[m.reported] = env.modelCostByDay(t, m.reported)
			}
		}

		row = priceRow(t, env.db, flash.reported, flash.vendor)
		_, err = env.svc.UpdateModelPriceAndRecalculate(row.ID, flash.reported, string(flash.vendor), row.CPT, row.CPIT, 0, row.CacheReadPT*1.0001, "USD")
		require.NoError(t, err)

		after := env.modelCostByDay(t, flash.reported)
		for d := 0; d < days; d++ {
			day := env.dayLabel(d)
			assertRel(t, before[day], after[day], 1e-4, "day %d moves by at most the cache price nudge", d)
		}
		for name, was := range others {
			assert.Equal(t, was, env.modelCostByDay(t, name), "%s is not repriced", name)
		}
	})
}

type chainTokens struct{ p, r, cw, cr int }

// Repricing p1 -> p2 keeps each row's ratio to the list price (sync 1x,
// batch 0.5x, flex 0.6x), and many repricings back to p2 leave no float
// drift.
func TestPricingRegression_RecalcRatiosAndDrift(t *testing.T) {
	forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
		svc := &services.Service{DB: db}
		vendor := string(models.ANTHROPIC)
		model := "claude-chain"
		p1 := models.ModelPrice{ModelName: model, Vendor: vendor, CPIT: 3e-6, CPT: 15e-6, CacheWritePT: 3.75e-6, CacheReadPT: 0.3e-6, Currency: "USD"}
		require.NoError(t, p1.Create(db))
		tk := chainTokens{1200, 300, 50, 900}
		list := func(p models.ModelPrice) float64 {
			return models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, tk.p, tk.r, tk.cw, tk.cr)
		}
		mults := map[string]float64{"sync": 1, "batch": 0.5, "flex": 0.6}
		ids := map[string]uint{}
		for name, mult := range mults {
			rec := models.LLMChatRecord{Name: model, Vendor: vendor, PromptTokens: tk.p, ResponseTokens: tk.r, CacheWritePromptTokens: tk.cw, CacheReadPromptTokens: tk.cr,
				Cost: list(p1) * mult, Currency: "USD", TimeStamp: time.Now()}
			require.NoError(t, db.Create(&rec).Error)
			ids[name] = rec.ID
		}
		cost := func(id uint) float64 {
			var r models.LLMChatRecord
			require.NoError(t, db.First(&r, id).Error)
			return r.Cost
		}
		reprice := func(p models.ModelPrice) {
			t.Helper()
			_, err := svc.UpdateModelPriceAndRecalculate(p1.ID, model, vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
			require.NoError(t, err)
		}

		p2 := p1
		p2.CPIT, p2.CPT = 2e-6, 10e-6
		reprice(p2)
		for name, mult := range mults {
			assertRel(t, list(p2)*mult, cost(ids[name]), 1e-12, "%s keeps its %.1fx ratio after p1 -> p2", name, mult)
		}

		for i := 0; i < 200; i++ {
			p := p2
			f := 1 + 0.37*math.Sin(float64(i))
			p.CPIT, p.CPT = p2.CPIT*f, p2.CPT*f
			reprice(p)
		}
		reprice(p2)
		for name, mult := range mults {
			assertRel(t, list(p2)*mult, cost(ids[name]), 1e-9, "%s after 200 repricings back to p2", name)
		}
	})
}

// An all-history recalculation is one set-based update: it reprices every
// row of the model within a time bound and leaves other models alone.
func TestPricingRegression_RecalcVolume(t *testing.T) {
	rows, bound := 5_000, 5*time.Second
	if pricingFull {
		rows, bound = 180_000, 30*time.Second
	}
	forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
		env := newStressEnvOn(t, db, 180)
		m, other := geminiMix[2], geminiMix[0]
		env.createPrices(t, []stressModel{m, other})
		// One recorded exchange each, copied across the history.
		proto := func(s stressModel) models.LLMChatRecord {
			resp := &MockTokenResponse{model: s.reported, promptTokens: s.prompt + s.cacheRead, responseTokens: s.response, cacheReadTokens: s.cacheRead}
			return *buildChatRecordFor(env.svc, env.llms[s.vendor], env.app, resp, s.requested, env.day0)
		}
		insert := func(r models.LLMChatRecord, n int) {
			step := 180 * 24 * time.Hour / time.Duration(n)
			batch := make([]models.LLMChatRecord, 0, 1000)
			for i := 0; i < n; i++ {
				r.TimeStamp = env.day0.Add(time.Duration(i) * step)
				batch = append(batch, r)
				if len(batch) == cap(batch) || i == n-1 {
					require.NoError(t, env.db.CreateInBatches(batch, 1000).Error)
					batch = batch[:0]
				}
			}
		}
		rec, otherRec := proto(m), proto(other)
		insert(rec, rows)
		insert(otherRec, 100)

		row := priceRow(t, env.db, m.reported, m.vendor)
		start := time.Now()
		_, err := env.svc.UpdateModelPriceAndRecalculate(row.ID, m.reported, string(m.vendor), m.price.CPT*2, m.price.CPIT*2, 0, 0, "USD")
		took := time.Since(start)
		require.NoError(t, err)
		t.Logf("recalculated %d rows in %s", rows, took)
		assert.Less(t, took, bound, "recalculating %d rows", rows)

		sum := func(name string) (n int64, total float64) {
			require.NoError(t, env.db.Model(&models.LLMChatRecord{}).Where("name = ?", name).Count(&n).Error)
			require.NoError(t, env.db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost), 0)").Where("name = ?", name).Scan(&total).Error)
			return n, total
		}
		n, total := sum(m.reported)
		assert.Equal(t, int64(rows), n)
		assertRel(t, rec.Cost*2*float64(rows), total, 1e-9, "every row repriced at the doubled price")
		n, total = sum(other.reported)
		assert.Equal(t, int64(100), n)
		assertRel(t, otherRec.Cost*100, total, 1e-12, "another model is not repriced")
	})
}

// findOrCreatePrice is the price row the admin edits for name: the one
// traffic already created, or a new one.
func (e *stressEnv) findOrCreatePrice(t *testing.T, name string, vendor models.Vendor) *models.ModelPrice {
	t.Helper()
	var p models.ModelPrice
	err := e.db.Where("model_name = ? AND vendor = ?", name, string(vendor)).First(&p).Error
	if err == nil {
		return &p
	}
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	created, err := e.svc.CreateModelPrice(name, string(vendor), 0, 0, 0, 0, "USD")
	require.NoError(t, err)
	return created
}

// kq3b.1: the gateway records traffic under the name the vendor reports
// (gemini-2.0-flash-001) and prices it by the requested name
// (gemini-2.0-flash) when the reported one is unpriced. Recalculating the
// requested name's price reprices that history too, on every day.
func TestPricingRegression_RecalcOnRequestedName(t *testing.T) {
	mixAll := append(append([]stressModel{}, geminiMix...), otherVendors[0]) // three versioned names
	cases := []struct {
		name string
		// pricedFrom is the price on the requested name while the history
		// is recorded (nil: unpriced, the customer case).
		pricedFrom func(m stressModel) *models.ModelPrice
	}{
		{"unpriced history, price set with recalculate", func(stressModel) *models.ModelPrice { return nil }},
		{"priced history, price changed with recalculate", func(m stressModel) *models.ModelPrice {
			p := m.price
			p.CPT, p.CPIT, p.CacheReadPT = p.CPT*3, p.CPIT*3, p.CacheReadPT*3
			return &p
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
				days, mix := pricingScale(mixAll)
				env := newStressEnvOn(t, db, days)
				for _, m := range mix {
					if p := c.pricedFrom(m); p != nil {
						_, err := env.svc.CreateModelPrice(m.requested, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
						require.NoError(t, err)
					}
				}
				env.recordDays(t, mix, 0, days)
				for _, m := range mix {
					if p := c.pricedFrom(m); p != nil {
						assertRel(t, m.dayCostAt(*p), env.modelCostByDay(t, m.reported)[env.dayLabel(0)], 1e-9,
							"%s recorded at the requested name's price", m.reported)
					}
				}

				for _, m := range mix {
					row := env.findOrCreatePrice(t, m.requested, m.vendor)
					p := m.price
					_, err := env.svc.UpdateModelPriceAndRecalculate(row.ID, m.requested, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
					require.NoError(t, err)
				}

				for _, m := range mix {
					got := env.modelCostByDay(t, m.reported)
					for d := 0; d < days; d++ {
						assertRel(t, m.dayCost(), got[env.dayLabel(d)], 1e-9, "%s on day %d after recalculating %s", m.reported, d, m.requested)
					}
				}
				assertRel(t, mixDayCost(mix)*float64(days), env.costByDay(t).total, 1e-9, "history total at the new prices")
			})
		})
	}
}

// kq3b.1 boundary: a reported name with a price of its own is priced by that
// row, so recalculating the requested name leaves it alone.
func TestPricingRegression_RecalcOnRequestedNameSkipsPricedReported(t *testing.T) {
	forEachPricingDB(t, func(t *testing.T, db *gorm.DB) {
		days, mix := pricingScale(otherVendors[:1]) // gpt-4o reported as gpt-4o-2024-08-06
		m := mix[0]
		env := newStressEnvOn(t, db, days)
		env.createPrices(t, mix) // on the reported name
		alias, err := env.svc.CreateModelPrice(m.requested, string(m.vendor), m.price.CPT*2, m.price.CPIT*2, 0, m.price.CacheReadPT*2, "USD")
		require.NoError(t, err)
		env.recordDays(t, mix, 0, days)

		_, err = env.svc.UpdateModelPriceAndRecalculate(alias.ID, m.requested, string(m.vendor), m.price.CPT*5, m.price.CPIT*5, 0, m.price.CacheReadPT*5, "EUR")
		require.NoError(t, err)

		got := env.modelCostByDay(t, m.reported)
		for d := 0; d < days; d++ {
			assertRel(t, m.dayCost(), got[env.dayLabel(d)], 1e-9, "day %d stays at the reported name's price", d)
		}
		var currencies []string
		require.NoError(t, env.db.Model(&models.LLMChatRecord{}).Distinct("currency").Where("name = ?", m.reported).Pluck("currency", &currencies).Error)
		assert.Equal(t, []string{"USD"}, currencies)
	})
}
