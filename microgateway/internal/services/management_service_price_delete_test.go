package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deleted price was soft-deleted, and the unique index on (model_name,
// vendor) still covered it, so the price could never be created again.
// Deletes are hard deletes now.
func TestManagementService_DeletedPriceCanBeCreatedAgain(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	req := &CreateModelPriceRequest{Vendor: "google_ai", ModelName: "gemini-2.5-flash", CPT: 0.000002, CPIT: 0.000001}
	price, err := service.CreateModelPrice(req)
	require.NoError(t, err)

	require.NoError(t, service.DeleteModelPrice(price.ID))
	var n int64
	require.NoError(t, db.Unscoped().Model(&database.ModelPrice{}).Count(&n).Error)
	assert.Zero(t, n, "no soft-deleted row is left behind")

	_, err = service.CreateModelPrice(req)
	require.NoError(t, err)
}

// Rows older versions soft-deleted are purged at migration, which runs on
// every start.
func TestMigrate_PurgesSoftDeletedModelPrices(t *testing.T) {
	db, _, _ := setupManagementTestDB(t)
	kept := database.ModelPrice{Vendor: "openai", ModelName: "gpt-4o", CPT: 1, CPIT: 1}
	gone := database.ModelPrice{Vendor: "openai", ModelName: "gpt-4.1", CPT: 1, CPIT: 1}
	require.NoError(t, db.Create(&kept).Error)
	require.NoError(t, db.Create(&gone).Error)
	require.NoError(t, db.Model(&gone).Update("deleted_at", gorm.DeletedAt{Time: time.Now(), Valid: true}).Error)

	require.NoError(t, database.Migrate(db))
	var names []string
	require.NoError(t, db.Unscoped().Model(&database.ModelPrice{}).Pluck("model_name", &names).Error)
	assert.Equal(t, []string{"gpt-4o"}, names)
	require.NoError(t, database.Migrate(db), "idempotent")
}
