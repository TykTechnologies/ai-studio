package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// Every user joins Default, always. Community Edition has no teams and
// Enterprise does, and universal Default membership is what keeps the two
// editions behaving the same way -- a user outside Default has no equivalent
// in CE. These pin that so it is not "tidied up" into a conditional later.
func TestApplyDefaultGroup(t *testing.T) {
	db := setupTestDBForSubmissions(t)
	service := NewService(db)

	t.Run("no teams specified joins Default", func(t *testing.T) {
		groups, err := service.applyDefaultGroup(nil)
		assert.NoError(t, err)
		assert.Contains(t, groups, models.DefaultGroupID)
	})

	t.Run("an explicit team choice still joins Default as well", func(t *testing.T) {
		group, err := service.CreateGroup("Support", []uint{}, []uint{}, []uint{}, []uint{})
		assert.NoError(t, err)

		groups, err := service.applyDefaultGroup([]uint{group.ID})
		assert.NoError(t, err)
		assert.Contains(t, groups, group.ID)
		assert.Contains(t, groups, models.DefaultGroupID,
			"Default membership is a CE/EE compatibility constant, not a default that can be opted out of")
	})

	t.Run("Default is not added twice", func(t *testing.T) {
		groups, err := service.applyDefaultGroup([]uint{models.DefaultGroupID})
		assert.NoError(t, err)
		assert.Equal(t, []uint{models.DefaultGroupID}, groups)
	})
}
