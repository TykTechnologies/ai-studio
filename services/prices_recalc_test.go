package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Repricing keeps each chat record's ratio of recorded cost to list cost, so
// batch rows (0.5x) are not re-billed at list price. Runs on SQLite and, when
// DATABASE_URL is set, on Postgres.

type recalcTokens struct{ prompt, response, cacheWrite, cacheRead int }

func (tk recalcTokens) listCost(vendor string, p models.ModelPrice) float64 {
	return models.ChatRecordCost(vendor, p.CPT, p.CPIT, p.CacheWritePT, p.CacheReadPT, tk.prompt, tk.response, tk.cacheWrite, tk.cacheRead)
}

func forEachRecalcDB(t *testing.T, run func(t *testing.T, s *Service, model string)) {
	t.Run("sqlite", func(t *testing.T) {
		run(t, setupTestService(t), "recalc-sqlite")
	})
	t.Run("postgres", func(t *testing.T) {
		s := setupPostgresServiceTest(t)
		model := fmt.Sprintf("recalc-pg-%d", time.Now().UnixNano())
		t.Cleanup(func() {
			s.DB.Exec("DELETE FROM llm_chat_records WHERE name LIKE ?", model+"%")
			s.DB.Unscoped().Exec("DELETE FROM model_prices WHERE model_name LIKE ?", model+"%")
		})
		run(t, s, model)
	})
}

func createRecalcRecord(t *testing.T, s *Service, model, vendor string, tk recalcTokens, cost float64) uint {
	t.Helper()
	rec := models.LLMChatRecord{
		Name: model, Vendor: vendor,
		PromptTokens: tk.prompt, ResponseTokens: tk.response,
		CacheWritePromptTokens: tk.cacheWrite, CacheReadPromptTokens: tk.cacheRead,
		TotalTokens: tk.prompt + tk.response + tk.cacheWrite + tk.cacheRead,
		Cost:        cost, Currency: "USD", TimeStamp: time.Now(),
	}
	require.NoError(t, s.DB.Create(&rec).Error)
	return rec.ID
}

func recordCost(t *testing.T, s *Service, id uint) models.LLMChatRecord {
	t.Helper()
	var got models.LLMChatRecord
	require.NoError(t, s.DB.First(&got, id).Error)
	return got
}

func TestUpdateModelPriceAndRecalculate_KeepsEachRowsPriceRatio(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.ANTHROPIC)
		old := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD",
			CPIT: 0.000003, CPT: 0.000015, CacheWritePT: 0.00000375, CacheReadPT: 0.0000003}
		require.NoError(t, old.Create(s.DB))
		updated := old
		updated.CPIT, updated.CPT, updated.CacheReadPT, updated.Currency = 0.000002, 0.000010, 0.0000002, "EUR"

		sync := recalcTokens{prompt: 1200, response: 300, cacheWrite: 50, cacheRead: 900}
		batch := recalcTokens{prompt: 5000, response: 2000, cacheWrite: 0, cacheRead: 100}
		syncID := createRecalcRecord(t, s, model, vendor, sync, sync.listCost(vendor, old))
		batchID := createRecalcRecord(t, s, model, vendor, batch, batch.listCost(vendor, old)*0.5)
		// Recorded with no cost (e.g. before the price existed): priced at 1x.
		unpricedID := createRecalcRecord(t, s, model, vendor, sync, 0)

		_, err := s.UpdateModelPriceAndRecalculate(old.ID, model, vendor, updated.CPT, updated.CPIT, updated.CacheWritePT, updated.CacheReadPT, "EUR")
		require.NoError(t, err)

		gotSync := recordCost(t, s, syncID)
		assert.InDelta(t, sync.listCost(vendor, updated), gotSync.Cost, 1e-9, "a list-price row gets exactly the new list price")
		assert.Equal(t, "EUR", gotSync.Currency)
		assert.InDelta(t, batch.listCost(vendor, updated)*0.5, recordCost(t, s, batchID).Cost, 1e-9, "a 0.5x row stays at 0.5x")
		assert.InDelta(t, sync.listCost(vendor, updated), recordCost(t, s, unpricedID).Cost, 1e-9, "a row with no recorded cost gets the new list price")
	})
}

func TestUpdateModelPriceAndRecalculate_ZeroOldListCostBillsAtNewList(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.ANTHROPIC)
		free := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD"}
		require.NoError(t, free.Create(s.DB))
		tk := recalcTokens{prompt: 100, response: 40}
		// Cost 7 against an old list cost of 0: there is no ratio to keep.
		id := createRecalcRecord(t, s, model, vendor, tk, 7)

		updated := models.ModelPrice{CPIT: 0.000004, CPT: 0.000012}
		_, err := s.UpdateModelPriceAndRecalculate(free.ID, model, vendor, updated.CPT, updated.CPIT, 0, 0, "USD")
		require.NoError(t, err)
		assert.InDelta(t, tk.listCost(vendor, updated), recordCost(t, s, id).Cost, 1e-9)
	})
}

// Renaming a price row reprices the records of the new name. They were not
// priced by this row, so they get the new list price at 1x.
func TestUpdateModelPriceAndRecalculate_RenameBillsAtNewList(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.ANTHROPIC)
		mp := models.ModelPrice{ModelName: model + "-old", Vendor: vendor, Currency: "USD", CPIT: 0.000001, CPT: 0.000002}
		require.NoError(t, mp.Create(s.DB))
		tk := recalcTokens{prompt: 100, response: 40}
		id := createRecalcRecord(t, s, model+"-new", vendor, tk, 3)

		updated := models.ModelPrice{CPIT: 0.000004, CPT: 0.000012}
		_, err := s.UpdateModelPriceAndRecalculate(mp.ID, model+"-new", vendor, updated.CPT, updated.CPIT, 0, 0, "USD")
		require.NoError(t, err)
		assert.InDelta(t, tk.listCost(vendor, updated), recordCost(t, s, id).Cost, 1e-9)
	})
}

// Records the vendor reported under a versioned name are priced by the
// requested name while the versioned name is unpriced (proxy
// priceForExchange), so recalculating the requested name reprices them too.
// Once the versioned name has a price of its own, they are its rows.
func TestUpdateModelPriceAndRecalculate_RepricesRowsRequestedByName(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.GOOGLEAI)
		alias := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD"}
		require.NoError(t, alias.Create(s.DB))
		unpricedVersion := models.ModelPrice{ModelName: model + "-001", Vendor: vendor, Currency: "USD"}
		require.NoError(t, unpricedVersion.Create(s.DB))
		pricedVersion := models.ModelPrice{ModelName: model + "-002", Vendor: vendor, Currency: "USD", CPIT: 0.000001, CPT: 0.000002}
		require.NoError(t, pricedVersion.Create(s.DB))

		tk := recalcTokens{prompt: 100, response: 40}
		requestedAs := func(name, requested string, cost float64) uint {
			id := createRecalcRecord(t, s, name, vendor, tk, cost)
			require.NoError(t, s.DB.Model(&models.LLMChatRecord{}).Where("id = ?", id).Update("requested_model", requested).Error)
			return id
		}
		viaUnpriced := requestedAs(model+"-001", model, 0)
		viaPriced := requestedAs(model+"-002", model, tk.listCost(vendor, pricedVersion))
		// Another model's row that merely shares the reported name is not
		// this row's to reprice.
		otherRequest := requestedAs(model+"-001", model+"-other", 0)
		// A row without a requested model (edge, older rows) matches by name only.
		legacy := createRecalcRecord(t, s, model+"-001", vendor, tk, 0)

		updated := models.ModelPrice{CPIT: 0.000004, CPT: 0.000012}
		_, err := s.UpdateModelPriceAndRecalculate(alias.ID, model, vendor, updated.CPT, updated.CPIT, 0, 0, "EUR")
		require.NoError(t, err)

		got := recordCost(t, s, viaUnpriced)
		assert.InDelta(t, tk.listCost(vendor, updated), got.Cost, 1e-9, "the versioned name is unpriced: priced by the requested name")
		assert.Equal(t, "EUR", got.Currency)
		assert.InDelta(t, tk.listCost(vendor, pricedVersion), recordCost(t, s, viaPriced).Cost, 1e-9, "the versioned name has its own price")
		assert.Zero(t, recordCost(t, s, otherRequest).Cost)
		assert.Zero(t, recordCost(t, s, legacy).Cost)
	})
}

// A row's price multiplier survives a recalculation through a zero price:
// stored on new rows, and recorded from the cost ratio on older rows the
// first time a recalculation can see it.
func TestUpdateModelPriceAndRecalculate_KeepsMultiplierThroughZeroPrice(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.ANTHROPIC)
		p1 := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD", CPIT: 0.000003, CPT: 0.000015}
		require.NoError(t, p1.Create(s.DB))
		p2 := models.ModelPrice{CPIT: 0.000002, CPT: 0.000010}

		tk := recalcTokens{prompt: 5000, response: 2000}
		stored := func(mult float64) uint {
			id := createRecalcRecord(t, s, model, vendor, tk, tk.listCost(vendor, p1)*mult)
			require.NoError(t, s.DB.Model(&models.LLMChatRecord{}).Where("id = ?", id).Update("price_multiplier", mult).Error)
			return id
		}
		batch := stored(0.5)
		free := stored(0)
		// An older row: no stored multiplier, billed at 0.6x.
		legacy := createRecalcRecord(t, s, model, vendor, tk, tk.listCost(vendor, p1)*0.6)

		reprice := func(p models.ModelPrice) {
			_, err := s.UpdateModelPriceAndRecalculate(p1.ID, model, vendor, p.CPT, p.CPIT, 0, 0, "USD")
			require.NoError(t, err)
		}
		reprice(p2)
		reprice(models.ModelPrice{})
		assert.Zero(t, recordCost(t, s, batch).Cost)
		reprice(p2)

		assert.InDelta(t, tk.listCost(vendor, p2)*0.5, recordCost(t, s, batch).Cost, 1e-9)
		assert.Zero(t, recordCost(t, s, free).Cost, "a 0x multiplier stays free")
		got := recordCost(t, s, legacy)
		assert.InDelta(t, tk.listCost(vendor, p2)*0.6, got.Cost, 1e-9, "the older row's ratio was recorded before the zero price")
		require.NotNil(t, got.PriceMultiplier)
		assert.InDelta(t, 0.6, *got.PriceMultiplier, 1e-12)
	})
}

// A recalculation keeps the price each row was recorded at relative to the
// list price, also for rows with a stored multiplier: rows recorded before a
// standard update (no recalculation) to half the price stay at twice the
// new list price. The ratio is stored, so it survives a zero price too.
func TestUpdateModelPriceAndRecalculate_KeepsRecordedPriceIntervals(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.ANTHROPIC)
		p := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD", CPIT: 0.000004, CPT: 0.000012}
		require.NoError(t, p.Create(s.DB))
		tk := recalcTokens{prompt: 1000, response: 100}
		one := 1.0
		early := createRecalcRecord(t, s, model, vendor, tk, tk.listCost(vendor, p))
		require.NoError(t, s.DB.Model(&models.LLMChatRecord{}).Where("id = ?", early).Update("price_multiplier", one).Error)

		half := models.ModelPrice{CPIT: p.CPIT / 2, CPT: p.CPT / 2}
		_, err := s.UpdateModelPrice(p.ID, model, vendor, half.CPT, half.CPIT, 0, 0, "USD")
		require.NoError(t, err)

		reprice := func(np models.ModelPrice) {
			_, err := s.UpdateModelPriceAndRecalculate(p.ID, model, vendor, np.CPT, np.CPIT, 0, 0, "USD")
			require.NoError(t, err)
		}
		reprice(half)
		assert.InDelta(t, tk.listCost(vendor, p), recordCost(t, s, early).Cost, 1e-9, "recorded at twice the new price, it stays there")
		reprice(models.ModelPrice{})
		reprice(half)
		assert.InDelta(t, tk.listCost(vendor, p), recordCost(t, s, early).Cost, 1e-9, "the ratio survives a zero price")
	})
}
