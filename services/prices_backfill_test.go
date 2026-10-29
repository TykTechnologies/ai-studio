package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Backfill prices only the history recorded at no cost, at the current
// price and each row's multiplier, over the rows the price prices
// (including those requested by its name and reported under an unpriced
// versioned name), within the optional date range.
func TestBackfillUnpricedHistory(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.GOOGLEAI)
		price := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "EUR", CPIT: 0.000001, CPT: 0.000004}
		require.NoError(t, price.Create(s.DB))
		require.NoError(t, (&models.ModelPrice{ModelName: model + "-001", Vendor: vendor, Currency: "USD"}).Create(s.DB))

		tk := recalcTokens{prompt: 1000, response: 100}
		day := func(d int) time.Time { return time.Date(2026, 5, d, 12, 0, 0, 0, time.UTC) }
		rec := func(name, requested string, cost float64, ts time.Time, mult *float64) uint {
			id := createRecalcRecord(t, s, name, vendor, tk, cost)
			require.NoError(t, s.DB.Model(&models.LLMChatRecord{}).Where("id = ?", id).
				Updates(map[string]any{"requested_model": requested, "time_stamp": ts, "price_multiplier": mult}).Error)
			return id
		}
		half := 0.5
		before := rec(model, model, 0, day(1), nil)
		inRange := rec(model, model, 0, day(10), nil)
		batch := rec(model, model, 0, day(10), &half)
		versioned := rec(model+"-001", model, 0, day(20), nil)
		lastDay := rec(model, model, 0, day(31), nil)
		alreadyPriced := rec(model, model, 3, day(10), nil)
		empty := createRecalcRecord(t, s, model, vendor, recalcTokens{}, 0)

		counts, err := s.CountUnpricedHistory(models.ModelPrices{price})
		require.NoError(t, err)
		assert.Equal(t, int64(5), counts[price.ID], "the unpriced rows a full backfill would price")

		from, to := day(2).Truncate(24*time.Hour), day(31).Truncate(24*time.Hour).AddDate(0, 0, 1)
		n, err := s.BackfillUnpricedHistory(price.ID, &from, &to)
		require.NoError(t, err)
		assert.Equal(t, int64(4), n)

		list := tk.listCost(vendor, price)
		assert.Zero(t, recordCost(t, s, before).Cost, "before the range")
		got := recordCost(t, s, inRange)
		assert.InDelta(t, list, got.Cost, 1e-9)
		assert.Equal(t, "EUR", got.Currency)
		assert.InDelta(t, list*0.5, recordCost(t, s, batch).Cost, 1e-9, "at the row's multiplier")
		assert.InDelta(t, list, recordCost(t, s, versioned).Cost, 1e-9, "requested by the name, reported under an unpriced version")
		assert.InDelta(t, list, recordCost(t, s, lastDay).Cost, 1e-9, "to is the end of its day")
		assert.Equal(t, 3.0, recordCost(t, s, alreadyPriced).Cost, "rows with a cost are left alone")
		assert.Zero(t, recordCost(t, s, empty).Cost)

		counts, err = s.CountUnpricedHistory(models.ModelPrices{price})
		require.NoError(t, err)
		assert.Equal(t, int64(1), counts[price.ID])

		n, err = s.BackfillUnpricedHistory(price.ID, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n, "no range: everything left")
		assert.InDelta(t, list, recordCost(t, s, before).Cost, 1e-9)

		_, err = s.BackfillUnpricedHistory(price.ID+1000, nil, nil)
		assert.Error(t, err)
	})
}
