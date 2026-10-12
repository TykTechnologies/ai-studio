package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cost breakdown of GetTotalCostPerVendorAndModel is computed in SQL; these
// tests pin it to the Go path that writes the recorded cost
// (models.ChatRecordCost, used by proxy.buildChatRecordFor). Each runs on
// SQLite and, when DATABASE_URL points at one, on Postgres.

var breakdownVendors = []models.Vendor{
	models.OPENAI, models.ANTHROPIC, models.VERTEX, models.GOOGLEAI,
	models.HUGGINGFACE, models.OLLAMA, models.BEDROCK, models.MOCK_VENDOR,
}

type breakdownTokens struct{ prompt, response, cacheWrite, cacheRead int }

var breakdownUsage = breakdownTokens{prompt: 1000, response: 500, cacheWrite: 200, cacheRead: 400}

// listComponents prices each token type on its own through the Go path, in
// the stored unit (currency x 10000).
func listComponents(vendor string, p models.ModelPrice, tk breakdownTokens) [4]float64 {
	cost := func(pt, rt, cw, cr int) float64 {
		return models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, pt, rt, cw, cr)
	}
	return [4]float64{
		cost(tk.prompt, 0, 0, 0),
		cost(0, tk.response, 0, 0),
		cost(0, 0, tk.cacheWrite, 0),
		cost(0, 0, 0, tk.cacheRead),
	}
}

func breakdownRecord(llmID uint, vendor, model string, tk breakdownTokens, cost float64, at time.Time) *models.LLMChatRecord {
	return &models.LLMChatRecord{
		LLMID: llmID, AppID: 1, Vendor: vendor, Name: model,
		PromptTokens: tk.prompt, ResponseTokens: tk.response,
		CacheWritePromptTokens: tk.cacheWrite, CacheReadPromptTokens: tk.cacheRead,
		Cost: cost, Currency: "USD", TimeStamp: at, InteractionType: models.ProxyInteraction,
	}
}

func breakdownRows(t *testing.T, db *gorm.DB, llmID uint, at time.Time) map[string]VendorModelCost {
	t.Helper()
	costs, err := GetTotalCostPerVendorAndModel(db, at.Add(-time.Hour), at.Add(time.Hour), nil, &llmID)
	require.NoError(t, err)
	rows := make(map[string]VendorModelCost, len(costs))
	for _, c := range costs {
		rows[c.Model] = c
	}
	return rows
}

// assertBreakdown compares a row's components (dollars) with want (stored unit)
// and checks they add up to the row's total.
func assertBreakdown(t *testing.T, row VendorModelCost, want [4]float64, label string) {
	t.Helper()
	got := [4]float64{row.PromptCost, row.ResponseCost, row.CacheWriteCost, row.CacheReadCost}
	names := [4]string{"prompt", "response", "cache write", "cache read"}
	for i := range got {
		assert.InDelta(t, want[i]/10000, got[i], 1e-12, "%s: %s cost", label, names[i])
	}
	assert.InDelta(t, row.TotalCost, got[0]+got[1]+got[2]+got[3], 1e-12, "%s: breakdown must add up to the total", label)
}

func forEachBreakdownDB(t *testing.T, run func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { run(t, setupStatsTest(t)) })
	t.Run("postgres", func(t *testing.T) {
		db := postgresSchemaDB(t)
		require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.ModelPrice{}))
		run(t, db)
	})
}

// With prices unchanged since recording, every vendor's components equal the
// Go path's per-token-type cost (so the ratio is 1 and sync rows are not
// rescaled), with and without cache prices set. A vendor whose unset cache
// price falls back to the input price in models.CachePricesFor must do so
// here too: editing one without the other fails this test.
func TestCostBreakdown_MatchesChatRecordCost(t *testing.T) {
	forEachBreakdownDB(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		const llmID = 7
		priceSets := map[string]models.ModelPrice{
			"unset-cache": {CPIT: 0.000010, CPT: 0.000030},
			"set-cache":   {CPIT: 0.000010, CPT: 0.000030, CacheWritePT: 0.000012, CacheReadPT: 0.000001},
		}
		type want struct {
			vendor     string
			components [4]float64
			cost       float64
		}
		wants := map[string]want{}
		for _, v := range breakdownVendors {
			vendor := string(v)
			for setName, p := range priceSets {
				model := vendor + "-" + setName
				p.ModelName, p.Vendor, p.Currency = model, vendor, "USD"
				require.NoError(t, db.Create(&p).Error)
				cost := models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT,
					breakdownUsage.prompt, breakdownUsage.response, breakdownUsage.cacheWrite, breakdownUsage.cacheRead)
				require.NoError(t, db.Create(breakdownRecord(llmID, vendor, model, breakdownUsage, cost, now)).Error)
				wants[model] = want{vendor: vendor, components: listComponents(vendor, p, breakdownUsage), cost: cost}
			}
		}

		rows := breakdownRows(t, db, llmID, now)
		require.Len(t, rows, len(wants))
		for model, w := range wants {
			row, ok := rows[model]
			require.True(t, ok, model)
			assert.InDelta(t, w.cost/10000, row.TotalCost, 1e-12, model)
			assertBreakdown(t, row, w.components, model)
		}

		// The cpit fallback is what makes OpenAI and Google cache cost non-zero
		// with unset cache prices; other vendors' unset cache prices stay free.
		assert.Greater(t, rows["openai-unset-cache"].CacheReadCost, 0.0)
		assert.Greater(t, rows["google_ai-unset-cache"].CacheWriteCost, 0.0)
		assert.Zero(t, rows["anthropic-unset-cache"].CacheReadCost)
	})
}

// A batch row recorded at 0.5x splits its halved cost in list proportions,
// and a group mixing a sync and a batch row sums each component.
func TestCostBreakdown_PriceMultiplier(t *testing.T) {
	forEachBreakdownDB(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		const llmID = 8
		vendor := string(models.ANTHROPIC)
		p := models.ModelPrice{ModelName: "claude-batch", Vendor: vendor, Currency: "USD",
			CPIT: 0.000003, CPT: 0.000015, CacheWritePT: 0.00000375, CacheReadPT: 0.0000003}
		require.NoError(t, db.Create(&p).Error)
		mixed := p
		mixed.ID, mixed.ModelName = 0, "claude-mixed"
		require.NoError(t, db.Create(&mixed).Error)

		list := listComponents(vendor, p, breakdownUsage)
		full := list[0] + list[1] + list[2] + list[3]
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "claude-batch", breakdownUsage, full*0.5, now)).Error)
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "claude-mixed", breakdownUsage, full, now)).Error)
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "claude-mixed", breakdownUsage, full*0.5, now)).Error)

		rows := breakdownRows(t, db, llmID, now)

		batch := rows["claude-batch"]
		assert.InDelta(t, full*0.5/10000, batch.TotalCost, 1e-12)
		var half, oneAndHalf [4]float64
		for i := range list {
			half[i] = list[i] * 0.5
			oneAndHalf[i] = list[i] * 1.5
		}
		assertBreakdown(t, batch, half, "batch")

		m := rows["claude-mixed"]
		assert.InDelta(t, full*1.5/10000, m.TotalCost, 1e-12)
		assertBreakdown(t, m, oneAndHalf, "mixed")
	})
}

// Decision: a row whose price changed after it was recorded keeps its
// recorded cost as the total, and its breakdown is that cost split in
// today's list-price proportions, so it still adds up.
func TestCostBreakdown_PriceChangedSinceRecording(t *testing.T) {
	forEachBreakdownDB(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		const llmID = 9
		vendor := string(models.ANTHROPIC)
		old := models.ModelPrice{ModelName: "claude-repriced", Vendor: vendor, Currency: "USD",
			CPIT: 0.000010, CPT: 0.000030, CacheWritePT: 0.000012, CacheReadPT: 0.000001}
		require.NoError(t, db.Create(&old).Error)
		oldList := listComponents(vendor, old, breakdownUsage)
		recorded := oldList[0] + oldList[1] + oldList[2] + oldList[3]
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "claude-repriced", breakdownUsage, recorded, now)).Error)

		// Reprice without recalculating the recorded cost.
		repriced := old
		repriced.CPIT, repriced.CPT = 0.000020, 0.000010
		require.NoError(t, db.Model(&models.ModelPrice{}).Where("id = ?", old.ID).
			Updates(map[string]any{"cpit": repriced.CPIT, "cpt": repriced.CPT}).Error)

		newList := listComponents(vendor, repriced, breakdownUsage)
		newTotal := newList[0] + newList[1] + newList[2] + newList[3]
		var want [4]float64
		for i := range newList {
			want[i] = newList[i] * recorded / newTotal
		}

		row := breakdownRows(t, db, llmID, now)["claude-repriced"]
		assert.InDelta(t, recorded/10000, row.TotalCost, 1e-12)
		assertBreakdown(t, row, want, "repriced")
	})
}

// A row with no model_prices entry, or one whose list cost is zero, does not
// error or divide by zero: its components stay zero and its total is kept.
func TestCostBreakdown_NoListCost(t *testing.T) {
	forEachBreakdownDB(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		const llmID = 10
		vendor := string(models.OPENAI)
		require.NoError(t, db.Create(&models.ModelPrice{ModelName: "free-model", Vendor: vendor, Currency: "USD"}).Error)
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "unpriced-model", breakdownUsage, 5, now)).Error)
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "free-model", breakdownUsage, 0, now)).Error)

		rows := breakdownRows(t, db, llmID, now)
		require.Len(t, rows, 2)
		unpriced := rows["unpriced-model"]
		assert.Nil(t, unpriced.ModelPriceID)
		assert.InDelta(t, 5.0/10000, unpriced.TotalCost, 1e-12)
		for _, row := range rows {
			assert.Zero(t, row.PromptCost+row.ResponseCost+row.CacheWriteCost+row.CacheReadCost, row.Model)
		}
	})
}

// After a price edit with recalculation, a batch row is still at 0.5x of the
// new list price and the breakdown still splits it in list proportions.
func TestCostBreakdown_AfterPriceRecalculation(t *testing.T) {
	forEachBreakdownDB(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		const llmID = 11
		vendor := string(models.ANTHROPIC)
		p := models.ModelPrice{ModelName: "claude-recalc", Vendor: vendor, Currency: "USD",
			CPIT: 0.000003, CPT: 0.000015, CacheWritePT: 0.00000375, CacheReadPT: 0.0000003}
		require.NoError(t, db.Create(&p).Error)
		old := listComponents(vendor, p, breakdownUsage)
		require.NoError(t, db.Create(breakdownRecord(llmID, vendor, "claude-recalc", breakdownUsage, (old[0]+old[1]+old[2]+old[3])*0.5, now)).Error)

		updated := p
		updated.CPIT, updated.CPT = 0.000002, 0.000010
		_, err := (&services.Service{DB: db}).UpdateModelPriceAndRecalculate(p.ID, p.ModelName, vendor,
			updated.CPT, updated.CPIT, updated.CacheWritePT, updated.CacheReadPT, "USD")
		require.NoError(t, err)

		list := listComponents(vendor, updated, breakdownUsage)
		var half [4]float64
		for i := range list {
			half[i] = list[i] * 0.5
		}
		row := breakdownRows(t, db, llmID, now)["claude-recalc"]
		assert.InDelta(t, (list[0]+list[1]+list[2]+list[3])*0.5/10000, row.TotalCost, 1e-12)
		assertBreakdown(t, row, half, "recalculated batch")
	})
}
