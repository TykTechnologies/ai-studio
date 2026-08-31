package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// Default owns catalogues holding every provider, tool and data source on the
// instance. Adding it on top of an explicit team choice granted access nobody
// granted, and failed in the permissive direction -- the wrong direction for a
// governance product.
func TestApplyDefaultGroup(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	t.Run("no teams specified keeps the Default onboarding behaviour", func(t *testing.T) {
		groups, err := service.applyDefaultGroup(nil)
		assert.NoError(t, err)
		assert.Contains(t, groups, models.DefaultGroupID)
	})

	t.Run("an empty slice is treated as no teams specified", func(t *testing.T) {
		groups, err := service.applyDefaultGroup([]uint{})
		assert.NoError(t, err)
		assert.Contains(t, groups, models.DefaultGroupID)
	})

	t.Run("an explicit team choice is respected exactly", func(t *testing.T) {
		group, err := service.CreateGroup("Support", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		groups, err := service.applyDefaultGroup([]uint{group.ID})
		assert.NoError(t, err)
		assert.Equal(t, []uint{group.ID}, groups)
		assert.NotContains(t, groups, models.DefaultGroupID)
	})

	t.Run("asking for Default explicitly still works", func(t *testing.T) {
		groups, err := service.applyDefaultGroup([]uint{models.DefaultGroupID})
		assert.NoError(t, err)
		assert.Equal(t, []uint{models.DefaultGroupID}, groups)
	})
}
