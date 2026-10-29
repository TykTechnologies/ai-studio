package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deleted price can be created again, by the admin and by the gateway's
// lookup, on SQLite and Postgres. A soft delete used to keep the row in
// idx_model_vendor, so both failed with a unique violation for good.
func TestDeleteModelPrice_PriceCanBeCreatedAgain(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.GOOGLEAI)
		mp, err := s.CreateModelPrice(model, vendor, 0.000002, 0.000001, 0, 0, "USD")
		require.NoError(t, err)
		require.NoError(t, s.DeleteModelPrice(mp.ID))

		var left int64
		require.NoError(t, s.DB.Unscoped().Model(&models.ModelPrice{}).Where("id = ?", mp.ID).Count(&left).Error)
		assert.Zero(t, left, "the delete is permanent")

		auto, err := s.GetModelPriceByModelNameAndVendor(model, vendor)
		require.NoError(t, err, "the gateway's lookup auto-creates the unpriced row again")
		assert.Zero(t, auto.CPIT)
		require.NoError(t, s.DeleteModelPrice(auto.ID))

		again, err := s.CreateModelPrice(model, vendor, 0.000004, 0.000003, 0, 0, "USD")
		require.NoError(t, err, "the admin can create the price again")
		got, err := s.GetModelPriceByModelNameAndVendor(model, vendor)
		require.NoError(t, err)
		assert.Equal(t, again.ID, got.ID)
		assert.Equal(t, 0.000003, got.CPIT)
	})
}

// Rows earlier versions soft-deleted are purged at migration.
func TestPurgeSoftDeletedModelPrices(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		vendor := string(models.GOOGLEAI)
		gone := models.ModelPrice{ModelName: model, Vendor: vendor, Currency: "USD", CPIT: 0.000001}
		require.NoError(t, gone.Create(s.DB))
		require.NoError(t, s.DB.Delete(&gone).Error) // a soft delete, as before
		kept := models.ModelPrice{ModelName: model + "-kept", Vendor: vendor, Currency: "USD", CPIT: 0.000001}
		require.NoError(t, kept.Create(s.DB))

		require.NoError(t, models.PurgeSoftDeletedModelPrices(s.DB))

		var n int64
		require.NoError(t, s.DB.Unscoped().Model(&models.ModelPrice{}).Where("id = ?", gone.ID).Count(&n).Error)
		assert.Zero(t, n)
		require.NoError(t, s.DB.Model(&models.ModelPrice{}).Where("id = ?", kept.ID).Count(&n).Error)
		assert.Equal(t, int64(1), n)
		_, err := s.CreateModelPrice(model, vendor, 0, 0.000002, 0, 0, "USD")
		require.NoError(t, err)
	})
}

// Concurrent first lookups of a model all get the one row: the loser of the
// insert race reads the winner's row instead of failing. Postgres only: the
// SQLite test database is in memory, one per connection.
func TestGetModelPriceByModelNameAndVendor_ConcurrentAutoCreate(t *testing.T) {
	forEachRecalcDB(t, func(t *testing.T, s *Service, model string) {
		if s.DB.Dialector.Name() != "postgres" {
			t.Skip("needs a database shared across connections")
		}
		vendor := string(models.GOOGLEAI)
		const n = 8
		ids := make(chan uint, n)
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			go func() {
				mp, err := s.GetModelPriceByModelNameAndVendor(model, vendor)
				if err != nil {
					errs <- err
					return
				}
				ids <- mp.ID
			}()
		}
		var first uint
		for i := 0; i < n; i++ {
			select {
			case err := <-errs:
				t.Fatalf("lookup failed: %v", err)
			case id := <-ids:
				if first == 0 {
					first = id
				}
				assert.Equal(t, first, id)
			}
		}
	})
}
