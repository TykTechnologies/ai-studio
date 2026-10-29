//go:build pricing_stress

package proxy

// Pricing stress diagnostics: run on demand (e.g. during pre-release), not
// part of CI. The build tag keeps them out of go test ./...
//
// Synthetic matrix: months of multi-vendor traffic recorded through the real
// buildChatRecordFor path, then price events at different points in time,
// then what the per-day cost chart shows. The scenarios log what they find
// rather than assert it; the logs are the report. See features/Pricing.md
// and the enterprise counterpart in enterprise/features/budget.
//
//   go test -tags pricing_stress ./proxy -run TestPricingStress -v -count=1 | grep pricing_stress_diag

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

type stressEnv struct {
	db   *gorm.DB
	svc  *services.Service
	llms map[models.Vendor]*models.LLM
	app  *models.App
	day0 time.Time // midnight UTC of the first day
	days int
}

func newStressEnv(t *testing.T, days int) *stressEnv {
	db := setupTestDB(t)
	env := &stressEnv{db: db, svc: &services.Service{DB: db}, llms: map[models.Vendor]*models.LLM{}, days: days}
	for _, v := range []models.Vendor{models.GOOGLEAI, models.OPENAI, models.ANTHROPIC} {
		llm := &models.LLM{Name: fmt.Sprintf("llm-%s", v), Vendor: v}
		require.NoError(t, db.Create(llm).Error)
		env.llms[v] = llm
	}
	env.app = &models.App{Name: "agent"}
	require.NoError(t, db.Create(env.app).Error)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	env.day0 = today.AddDate(0, 0, -(days - 1))
	return env
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
	pt := models.PromptTokensExcludingCache(string(m.vendor), m.prompt+m.cacheRead, 0, m.cacheRead)
	c := models.ChatRecordCost(string(m.vendor), m.price.CPT, m.price.CPIT, m.price.CacheWritePT, m.price.CacheReadPT, pt, m.response, 0, m.cacheRead)
	return c * float64(m.perDay) / 10000
}

func priceRow(t *testing.T, db *gorm.DB, name string, vendor models.Vendor) *models.ModelPrice {
	t.Helper()
	var p models.ModelPrice
	require.NoError(t, db.Where("model_name = ? AND vendor = ?", name, string(vendor)).First(&p).Error)
	return &p
}

type dailyCost struct {
	byDay map[string]float64
	total float64
}

// costByDay is GetCostAnalysis (the dashboard's cost chart) folded over currencies.
func (e *stressEnv) costByDay(t *testing.T) dailyCost {
	t.Helper()
	chart, err := analytics.GetCostAnalysis(e.db, e.day0, e.day0.AddDate(0, 0, e.days+1), nil)
	require.NoError(t, err)
	out := dailyCost{byDay: map[string]float64{}}
	for _, c := range chart {
		for i, l := range c.Labels {
			out.byDay[l] += c.Data[i]
			out.total += c.Data[i]
		}
	}
	return out
}

func (d dailyCost) summary(lastDay string) string {
	last := d.byDay[lastDay]
	nonZero := 0
	for _, v := range d.byDay {
		if v > 0 {
			nonZero++
		}
	}
	share := 0.0
	if d.total > 0 {
		share = last / d.total * 100
	}
	return fmt.Sprintf("total=$%.2f, days with cost=%d, last day=$%.2f (%.1f%% of total)", d.total, nonZero, last, share)
}

func (e *stressEnv) lastDay() string { return e.day0.AddDate(0, 0, e.days-1).Format("2006-01-02") }

func mixDayCost(mix []stressModel) float64 {
	s := 0.0
	for _, m := range mix {
		s += m.dayCost()
	}
	return s
}

// TestPricingStress_UnpricedThenPricedToday reproduces the reported case: an
// agent runs a Gemini mix for 6 months with no prices, the admin adds prices
// today, then today's traffic arrives. Each admin action the UI and API offer
// is a row of the matrix.
func TestPricingStress_UnpricedThenPricedToday(t *testing.T) {
	const days = 180
	type action struct {
		name string
		// nameFor picks the price row the admin edits: the model they asked
		// for, or the versioned one the vendor reported.
		nameFor     func(m stressModel) string
		recalculate bool
		create      bool
	}
	actions := []action{
		{"update requested-name row (standard)", func(m stressModel) string { return m.requested }, false, false},
		{"update requested-name row + recalculate", func(m stressModel) string { return m.requested }, true, false},
		{"update reported-name row (standard)", func(m stressModel) string { return m.reported }, false, false},
		{"update reported-name row + recalculate", func(m stressModel) string { return m.reported }, true, false},
		{"POST create price for requested name", func(m stressModel) string { return m.requested }, false, true},
	}
	for _, a := range actions {
		t.Run(a.name, func(t *testing.T) {
			env := newStressEnv(t, days)
			env.recordDays(t, geminiMix, 0, days-1)

			var autoRows []string
			var ps []models.ModelPrice
			require.NoError(t, env.db.Order("model_name").Find(&ps).Error)
			for _, p := range ps {
				autoRows = append(autoRows, fmt.Sprintf("%s(cpit=%g)", p.ModelName, p.CPIT))
			}
			t.Logf("price rows auto-created by traffic before the admin acts: %s", strings.Join(autoRows, ", "))

			for _, m := range geminiMix {
				p := m.price
				name := a.nameFor(m)
				if a.create {
					_, err := env.svc.CreateModelPrice(name, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
					t.Logf("create %s: err=%v", name, err)
					continue
				}
				row := priceRow(t, env.db, name, m.vendor)
				var err error
				if a.recalculate {
					_, err = env.svc.UpdateModelPriceAndRecalculate(row.ID, name, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
				} else {
					_, err = env.svc.UpdateModelPrice(row.ID, name, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
				}
				require.NoError(t, err)
			}
			before := env.costByDay(t)
			env.record(t, geminiMix, days-1) // today's traffic after pricing
			after := env.costByDay(t)

			want := mixDayCost(geminiMix)
			t.Logf("expected if history were priced: $%.2f/day, $%.2f over %d days", want, want*days, days)
			t.Logf("right after the price action: %s", before.summary(env.lastDay()))
			t.Logf("after today's traffic:        %s", after.summary(env.lastDay()))
			for _, m := range geminiMix {
				var n int64
				var c float64
				env.db.Model(&models.LLMChatRecord{}).Where("name = ? AND time_stamp < ?", m.reported, env.day0.AddDate(0, 0, days-1)).Count(&n)
				env.db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost),0)").Where("name = ? AND time_stamp < ?", m.reported, env.day0.AddDate(0, 0, days-1)).Scan(&c)
				t.Logf("  history %-32s rows=%d cost=$%.2f (expected $%.2f)", m.reported, n, c/10000, m.dayCost()*float64(days-1))
			}
		})
	}
}

// TestPricingStress_PriceChangesAcrossIntervals runs a 6-month multi-vendor
// mix with a vendor price change in the middle, then shows what an
// all-history recalculation does to each interval.
func TestPricingStress_PriceChangesAcrossIntervals(t *testing.T) {
	const days = 180
	mix := append(append([]stressModel{}, geminiMix...), otherVendors...)
	env := newStressEnv(t, days)
	// All priced from day 0 at their real price, on the row the gateway
	// prices by (the reported name), as an admin who got it right would.
	for _, m := range mix {
		p := m.price
		_, err := env.svc.CreateModelPrice(m.reported, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
		require.NoError(t, err)
	}
	env.recordDays(t, mix, 0, 90)
	// Day 90: Gemini 2.5 flash halves its price; admin uses the standard update.
	flash := mix[1]
	row := priceRow(t, env.db, flash.reported, flash.vendor)
	_, err := env.svc.UpdateModelPrice(row.ID, flash.reported, string(flash.vendor), flash.price.CPT/2, flash.price.CPIT/2, 0, flash.price.CacheReadPT/2, "USD")
	require.NoError(t, err)
	env.recordDays(t, mix, 90, days)

	monthTotals := func() map[string]float64 {
		var rows []struct {
			Month string
			Cost  float64
		}
		require.NoError(t, env.db.Raw(`SELECT strftime('%Y-%m', time_stamp) AS month, SUM(cost)/10000 AS cost FROM llm_chat_records WHERE name = ? GROUP BY month ORDER BY month`, flash.reported).Scan(&rows).Error)
		out := map[string]float64{}
		for _, r := range rows {
			out[r.Month] = r.Cost
		}
		return out
	}
	beforeRecalc := monthTotals()

	// Later the admin notices a cache price was wrong and "fixes" it with
	// recalculate, keeping the current (halved) list price.
	row = priceRow(t, env.db, flash.reported, flash.vendor)
	_, err = env.svc.UpdateModelPriceAndRecalculate(row.ID, flash.reported, string(flash.vendor), row.CPT, row.CPIT, 0, row.CacheReadPT*1.0001, "USD")
	require.NoError(t, err)
	afterRecalc := monthTotals()
	var months []string
	for m := range beforeRecalc {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		t.Logf("  %s %s: before recalc $%.2f  after recalc $%.2f  (%+.1f%%)", flash.reported, m, beforeRecalc[m], afterRecalc[m], (afterRecalc[m]/beforeRecalc[m]-1)*100)
	}
}

// TestPricingStress_RecalcChains walks one model through a sequence of
// recalculations and compares each row with its exact cost.
func TestPricingStress_RecalcChains(t *testing.T) {
	db := setupTestDB(t)
	svc := &services.Service{DB: db}
	vendor := string(models.ANTHROPIC)
	model := "claude-chain"
	p1 := models.ModelPrice{ModelName: model, Vendor: vendor, CPIT: 3e-6, CPT: 15e-6, CacheWritePT: 3.75e-6, CacheReadPT: 0.3e-6, Currency: "USD"}
	require.NoError(t, p1.Create(db))
	tk := struct{ p, r, cw, cr int }{1200, 300, 50, 900}
	list := func(p models.ModelPrice) float64 {
		return models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, tk.p, tk.r, tk.cw, tk.cr)
	}
	mk := func(mult float64) uint {
		rec := models.LLMChatRecord{Name: model, Vendor: vendor, PromptTokens: tk.p, ResponseTokens: tk.r, CacheWritePromptTokens: tk.cw, CacheReadPromptTokens: tk.cr, Cost: list(p1) * mult, Currency: "USD", TimeStamp: time.Now()}
		require.NoError(t, db.Create(&rec).Error)
		return rec.ID
	}
	sync, batch, flex := mk(1), mk(0.5), mk(0.6)
	cost := func(id uint) float64 {
		var r models.LLMChatRecord
		require.NoError(t, db.First(&r, id).Error)
		return r.Cost
	}
	step := func(label string, p models.ModelPrice) {
		_, err := svc.UpdateModelPriceAndRecalculate(p1.ID, model, vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
		require.NoError(t, err)
		l := list(p)
		ratio := func(id uint) string {
			if l == 0 {
				return fmt.Sprintf("%.4f", cost(id))
			}
			return fmt.Sprintf("%.4fx", cost(id)/l)
		}
		t.Logf("  %-34s sync=%s batch(0.5x)=%s flex(0.6x)=%s", label, ratio(sync), ratio(batch), ratio(flex))
	}
	p2 := p1
	p2.CPIT, p2.CPT = 2e-6, 10e-6
	step("p1 -> p2", p2)
	zero := p1
	zero.CPIT, zero.CPT, zero.CacheWritePT, zero.CacheReadPT = 0, 0, 0, 0
	step("p2 -> 0 (price cleared)", zero)
	step("0 -> p2 (price restored)", p2)
	inOnly := p2
	inOnly.CPT = 0
	inOnly.CacheWritePT, inOnly.CacheReadPT = 0, 0
	step("p2 -> output price 0 only", inOnly)
	step("-> p2 again", p2)

	// Drift over many small repricings.
	exact := list(p2)
	for i := 0; i < 200; i++ {
		p := p2
		f := 1 + 0.37*math.Sin(float64(i))
		p.CPIT, p.CPT = p2.CPIT*f, p2.CPT*f
		_, err := svc.UpdateModelPriceAndRecalculate(p1.ID, model, vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, "USD")
		require.NoError(t, err)
	}
	_, err := svc.UpdateModelPriceAndRecalculate(p1.ID, model, vendor, p2.CPT, p2.CPIT, p2.CacheWritePT, p2.CacheReadPT, "USD")
	require.NoError(t, err)
	t.Logf("  after 200 repricings back to p2: sync rel. error %.2e", math.Abs(cost(sync)-exact)/exact)
}

// TestPricingStress_DeletedPrice deletes a price and keeps sending traffic.
func TestPricingStress_DeletedPrice(t *testing.T) {
	env := newStressEnv(t, 3)
	m := geminiMix[0]
	p := m.price
	mp, err := env.svc.CreateModelPrice(m.reported, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
	require.NoError(t, err)
	env.record(t, []stressModel{m}, 0)
	require.NoError(t, env.svc.DeleteModelPrice(mp.ID))
	env.record(t, []stressModel{m}, 1)
	_, lookupErr := env.svc.GetModelPriceByModelNameAndVendor(m.reported, string(m.vendor))
	_, recreateErr := env.svc.CreateModelPrice(m.reported, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
	env.record(t, []stressModel{m}, 2)
	d := env.costByDay(t)
	for i := 0; i < 3; i++ {
		day := env.day0.AddDate(0, 0, i).Format("2006-01-02")
		t.Logf("  day %d cost $%.2f", i, d.byDay[day])
	}
	t.Logf("  lookup after delete: err=%v", lookupErr)
	t.Logf("  re-create after delete: err=%v", recreateErr)
}

// TestPricingStress_CurrencyMix: per-currency chart vs budget totals.
func TestPricingStress_CurrencyMix(t *testing.T) {
	env := newStressEnv(t, 30)
	mix := []stressModel{geminiMix[0], otherVendors[1]} // USD + EUR
	for _, m := range mix {
		p := m.price
		_, err := env.svc.CreateModelPrice(m.reported, string(m.vendor), p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, p.Currency)
		require.NoError(t, err)
	}
	env.recordDays(t, mix, 0, 30)
	chart, err := analytics.GetCostAnalysis(env.db, env.day0, env.day0.AddDate(0, 0, 31), nil)
	require.NoError(t, err)
	for cur, c := range chart {
		s := 0.0
		for _, v := range c.Data {
			s += v
		}
		t.Logf("  cost chart %s: %.2f", cur, s)
	}
	var appTotal float64
	env.db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost),0)/10000").Where("app_id = ?", env.app.ID).Scan(&appTotal)
	t.Logf("  App spend as every budget sums it (currency-blind): %.2f", appTotal)
}

// TestPricingStress_RecalcVolume times an all-history recalculation.
func TestPricingStress_RecalcVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("volume")
	}
	const days = 180
	env := newStressEnv(t, days)
	m := geminiMix[2]
	m.perDay = 1000 // 180k rows of one model
	env.recordDays(t, []stressModel{m}, 0, days)
	row := priceRow(t, env.db, m.reported, m.vendor)
	start := time.Now()
	_, err := env.svc.UpdateModelPriceAndRecalculate(row.ID, m.reported, string(m.vendor), m.price.CPT, m.price.CPIT, 0, 0, "USD")
	require.NoError(t, err)
	t.Logf("  recalculated %d rows in %s (one transaction, SQLite)", days*m.perDay, time.Since(start))
}
