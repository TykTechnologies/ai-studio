//go:build !enterprise

package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In a Community build every way of getting the entitlements service (the
// constructor, a hand-built Service, a nil Service) yields the stub, and the
// stub's guard writes through the transaction it is given and passes fn's
// error back untouched, so wrapped call sites behave exactly as before.
func TestEntitlements_CommunityWiringIsTheStub(t *testing.T) {
	db := setupUserTestDB(t)

	built := NewService(db)
	require.NotNil(t, built.Entitlements)
	assert.Equal(t, entitlements.NewCommunityService(), built.EntitlementsService())

	hand := &Service{DB: db}
	got := hand.EntitlementsService()
	require.NotNil(t, got)
	assert.Equal(t, entitlements.NewCommunityService(), got)
	assert.Equal(t, got, hand.Entitlements, "kept on the Service")

	var nilSvc *Service
	assert.Equal(t, entitlements.NewCommunityService(), nilSvc.EntitlementsService())

	// A write inside the guard lands, and a failing fn rolls back with its
	// own error when the caller runs the guard in a transaction.
	err := db.Transaction(func(tx *gorm.DB) error {
		return got.GuardAppCreate(tx, 1, true, func(inner *gorm.DB) error {
			return inner.Create(&models.App{Name: "kept", UserID: 1}).Error
		})
	})
	require.NoError(t, err)

	boom := errors.New("boom")
	err = db.Transaction(func(tx *gorm.DB) error {
		return got.GuardUserChange(tx, func(inner *gorm.DB) error {
			if err := inner.Create(&models.App{Name: "rolled-back", UserID: 1}).Error; err != nil {
				return err
			}
			return boom
		})
	})
	assert.ErrorIs(t, err, boom)

	var names []string
	require.NoError(t, db.Model(&models.App{}).Order("id").Pluck("name", &names).Error)
	assert.Equal(t, []string{"kept"}, names)
}
